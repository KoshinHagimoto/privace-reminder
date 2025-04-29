package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/line/line-bot-sdk-go/v8/linebot"
)

func main() {
	// 1. チャンネルシークレットとアクセストークンを環境変数から取得
	channelSecret := os.Getenv("LINE_CHANNEL_SECRET")
	channelToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")

	// 2. LINE Bot クライアントを初期化
	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		log.Fatalf("LINE Bot 初期化エラー: %v", err)
	}

	// 3. /callback エンドポイントをハンドラ登録
	http.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		// 4. リクエストからイベントをパース
		events, err := bot.ParseRequest(req)
		if err != nil {
			if err == linebot.ErrInvalidSignature {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		// 5. 各イベントをループ処理
		for _, event := range events {
			// メッセージイベントかつテキストメッセージのみ処理
			if event.Type == linebot.EventTypeMessage {
				if _, ok := event.Message.(*linebot.TextMessage); ok {
					// 6. "hello world" を返信
					if _, err := bot.ReplyMessage(
						event.ReplyToken,
						linebot.NewTextMessage("hello world"),
					).Do(); err != nil {
						log.Printf("返信エラー: %v", err)
					}
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}) // :contentReference[oaicite:4]{index=4}

	// 7. サーバ起動（Heroku の場合、ポートは環境変数 PORT から取得）
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("サーバ起動: :%s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
