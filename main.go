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
			msg := fmt.Sprintf("%s の PRiVACE 特急予約はこちら！%s", target.Format("2006年1月2日"), url)
			if _, err := bot.BroadcastMessage(linebot.NewTextMessage(msg)).Do(); err != nil {
				log.Printf("BroadCast メッセージエラー(7:45): %v", err)
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
			msg := fmt.Sprintf("%s の PRiVACE 特急予約はこちら！%s", target.Format("2006年1月2日"), url)
			if _, err := bot.BroadcastMessage(linebot.NewTextMessage(msg)).Do(); err != nil {
				log.Printf("BroadCast メッセージエラー(23:55): %v", err)
			}
		}
	})
	c.Start()

	// 5. Webhook ハンドラ
	http.HandleFunc("/callback", callbackHandler(bot))

	// 6. テスト用フォームと /test
	http.HandleFunc("/form", formHandler)
	http.HandleFunc("/test", testHandler(loc))

	// 8. サーバ起動
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("サーバ起動: :%s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

// callbackHandler は Webhook 受信処理を返す
func callbackHandler(bot *linebot.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := bot.ParseRequest(r)
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
				if _, ok := event.Message.(*linebot.TextMessage); ok {
					defaultMsg := "PRiVACE座席予約サイトはこちら！\nhttps://privace.hankyu.co.jp/order/search.html"
					bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(defaultMsg)).Do()
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// formHandler は /form で入力フォームを表示
func formHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
			<html><body>
			<form action="/test" method="get">
			Date (YYYY-MM-DD): <input type="date" name="date" value="2025-05-13" /><br>
			Hour (HH): <input type="text" name="hour" value="07" size="2" /><br>
			Minute (MM): <input type="text" name="minute" value="40" size="2" /><br>
			<input type="submit" value="Check train.html" />
			</form>
			</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// testHandler は /test で実際の train.html 取得URLを返す
func testHandler(loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("date")
		if q == "" {
			http.Error(w, "date query parameter required", http.StatusBadRequest)
			return
		}
		target, err := time.ParseInLocation("2006-01-02", q, loc)
		if err != nil {
			http.Error(w, "invalid date format", http.StatusBadRequest)
			return
		}
		hour := r.URL.Query().Get("hour")
		minute := r.URL.Query().Get("minute")
		if hour == "" {
			hour = "07"
		}
		if minute == "" {
			minute = "40"
		}
		url, err := fetchTrainURL(target, KATSURA, OSAKAUMEDA, hour, minute)
		if err != nil {
			http.Error(w, fmt.Sprintf("fetchTrainURL error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Write([]byte(url))
	}
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
func fetchTrainURL(target time.Time, from, to, hour, minute string) (string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
	}

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
	data.Set("searchForm:departure", "桂")
	data.Set("searchForm:destination", "大阪梅田")
	data.Set("searchForm:doSearch", "検索する")

	// POST 実行
	req, _ := http.NewRequest(
		http.MethodPost,
		"https://privace.hankyu.co.jp/order/search.html",
		strings.NewReader(data.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PrivaceBot/1.0)")
	req.Header.Set("Referer", "https://privace.hankyu.co.jp/order/search.html")

	resp2, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	doc2, err := goquery.NewDocumentFromReader(resp2.Body)
	if err != nil {
		return "", err
	}

	if metaURL, exists := doc2.Find(`meta[property="og:url"]`).Attr("content"); exists {
		log.Println("meta URL found:", metaURL)
		return metaURL, nil
	}

	if action, ok := doc2.Find("form#trainForm").Attr("action"); ok {
		log.Println("action found trainForm: ", action)
		return "https://privace.hankyu.co.jp" + action, nil
	}

	if action, exists := doc2.Find("form#header1Form").Attr("action"); exists {
		// 絶対 URL を組み立て
		log.Println("action found header1Form: ", action)
		return "https://privace.hankyu.co.jp" + action, nil
	}

	// リダイレクト先 URL
	return resp2.Request.URL.String(), nil
}
