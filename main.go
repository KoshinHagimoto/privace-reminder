package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	holiday "github.com/holiday-jp/holiday_jp-go"
	"github.com/line/line-bot-sdk-go/v8/linebot"
	cron "github.com/robfig/cron/v3"
)

const (
	OSAKAUMEDA = "003450"
	KATSURA    = "003970"
)

func main() {
	// 1. 環境変数から LINE Bot シークレット・トークン・ユーザーIDを取得
	channelSecret := os.Getenv("LINE_CHANNEL_SECRET")
	channelToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	userID := os.Getenv("LINE_USER_ID")

	// 2. LINE Bot クライアントを初期化
	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		log.Fatalf("LINE Bot 初期化エラー: %v", err)
	}

	// 3. Cron スケジューラを日本時間で生成
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	c := cron.New(cron.WithLocation(loc))

	// 4. 毎日 7:45 にリマインド
	c.AddFunc("45 7 * * *", func() {
		now := time.Now().In(loc)
		target := now.AddDate(0, 0, 14)
		if isBusinessDay(target) {
			url, err := fetchTrainURL(target, KATSURA, OSAKAUMEDA, "07", "40")
			if err != nil {
				log.Printf("URL取得失敗(7:45): %v", err)
				return
			}
			msg := fmt.Sprintf("%s の PRiVACE 特急予約はこちら！%s", target.Format("2006-01-02"), url)
			if _, err := bot.PushMessage(userID, linebot.NewTextMessage(msg)).Do(); err != nil {
				log.Printf("Push メッセージエラー(7:45): %v", err)
			}
		}
	})

	// 5. 毎日 23:55 にリマインド
	c.AddFunc("55 23 * * *", func() {
		now := time.Now().In(loc)
		target := now.AddDate(0, 0, 15)
		if isBusinessDay(target) {
			url, err := fetchTrainURL(target, KATSURA, OSAKAUMEDA, "07", "40")
			if err != nil {
				log.Printf("URL取得失敗(23:55): %v", err)
				return
			}
			msg := fmt.Sprintf("%s の PRiVACE 特急予約はこちら！%s", target.Format("2006-01-02"), url)
			if _, err := bot.PushMessage(userID, linebot.NewTextMessage(msg)).Do(); err != nil {
				log.Printf("Push メッセージエラー(23:55): %v", err)
			}
		}
	})
	c.Start()

	// 6. Webhook ハンドラ (/callback)
	http.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		events, err := bot.ParseRequest(req)
		if err != nil {
			if err == linebot.ErrInvalidSignature {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		for _, event := range events {
			if event.Type == linebot.EventTypeMessage {
				switch event.Message.(type) {
				case *linebot.TextMessage:
					// デフォルトメッセージを変更
					defaultMsg := "PRiVACE座席予約サイトはこちら！\nhttps://privace.hankyu.co.jp/order/search.html"
					if _, err := bot.ReplyMessage(
						event.ReplyToken,
						linebot.NewTextMessage(defaultMsg),
					).Do(); err != nil {
						log.Printf("Reply エラー: %v", err)
					}
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	// 7. ヘルスチェックエンドポイント
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 8. サーバ起動
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("サーバ起動: :%s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

// isBusinessDay: 平日かつ祝日判定
func isBusinessDay(t time.Time) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	if holiday.IsHoliday(t) {
		return false
	}
	return true
}

// fetchTrainURL: 指定の検索条件で train.html へ遷移後の URL を返す
func fetchTrainURL(
	target time.Time,
	from, to, hour, minute string,
) (string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// GET search.html
	resp, err := client.Get("https://privace.hankyu.co.jp/order/search.html")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// フォームの hidden input を収集
	data := url.Values{}
	doc.Find("form#searchForm input").Each(func(_ int, s *goquery.Selection) {
		if name, ok := s.Attr("name"); ok && name != "" {
			val, _ := s.Attr("value")
			data.Set(name, val)
		}
	})

	// パラメータ上書き
	dateStr := target.Format("2006/01/02")
	data.Set("searchForm:fromStation", from)
	data.Set("searchForm:toStation", to)
	data.Set("searchForm:rideDate", dateStr)
	data.Set("searchForm:baseTimeHours", hour)
	data.Set("searchForm:baseTimeMinutes", minute)
	data.Set("searchForm:isDepartureBase", "true")
	data.Set("searchForm:isArrivalBase", "false")

	// POST 実行
	req, _ := http.NewRequest(
		http.MethodPost,
		"https://privace.hankyu.co.jp/order/search.html",
		strings.NewReader(data.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp2, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	// リダイレクト先 URL
	return resp2.Request.URL.String(), nil
}
