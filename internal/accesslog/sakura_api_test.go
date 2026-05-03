package accesslog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/azuki774/crd-sakura-simple-monitor/internal/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	client "github.com/sacloud/api-client-go"
)

var _ = Describe("SakuraCloud API access log", func() {
	DescribeTable("records one structured access log per API call",
		func(tt accessLogCase) {
			// API アクセスログは token や body を出さず、method/URI/時間/結果だけを残す。
			var records bytes.Buffer
			ctx := context.Background()
			log := logger.NewJSONLogger(&records, slog.LevelInfo)

			caller := NewSakuraAPICallerWithOptions(&client.Options{
				AccessToken:       "token",
				AccessTokenSecret: "secret",
				HttpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					if tt.err != nil {
						return nil, tt.err
					}
					return &http.Response{
						StatusCode: tt.statusCode,
						Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
					}, nil
				})},
			}, log)
			now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
			caller.setNowForTest(func() time.Time {
				defer func() { now = now.Add(150 * time.Millisecond) }()
				return now
			})

			data, err := caller.Do(ctx, http.MethodGet, "https://secure.sakura.ad.jp/cloud/zone/is1a/api/cloud/1.1/simplemonitor?token=secret", nil)
			tt.wantResult(data, err)

			record := records.String()
			Expect(strings.Split(strings.TrimSpace(record), "\n")).To(HaveLen(1))
			Expect(record).To(ContainSubstring(tt.wantMessage))
			Expect(record).To(ContainSubstring(`"method":"GET"`))
			Expect(record).To(ContainSubstring(`"uri":"https://secure.sakura.ad.jp/cloud/zone/is1a/api/cloud/1.1/simplemonitor"`))
			Expect(record).NotTo(ContainSubstring("token=secret"))
			Expect(record).To(ContainSubstring(`"duration":"150ms"`))
			Expect(record).To(ContainSubstring(`"statusCode":` + tt.wantStatusCode))
			Expect(record).To(ContainSubstring(`"logger":"sakura-api"`))
			tt.wantLog(record)
		},
		// 成功時はレスポンス本文を出さず、サイズだけを記録する。
		Entry("success", accessLogCase{
			statusCode:     http.StatusOK,
			responseBody:   "response",
			wantStatusCode: "200",
			wantMessage:    "SakuraCloud API access succeeded",
			wantResult: func(data []byte, err error) {
				Expect(err).NotTo(HaveOccurred())
				Expect(data).To(Equal([]byte("response")))
			},
			wantLog: func(record string) {
				Expect(record).To(ContainSubstring(`"responseBytes":8`))
			},
		}),
		// APIError の場合は statusCode/errorCode/serial を追加し、障害調査に使える形で残す。
		Entry("api error", accessLogCase{
			statusCode:     http.StatusForbidden,
			responseBody:   `{"serial":"serial-123","error_code":"forbidden","error_msg":"forbidden"}`,
			wantStatusCode: "403",
			wantMessage:    "SakuraCloud API access failed",
			wantResult: func(data []byte, err error) {
				Expect(data).To(BeNil())
				Expect(err).To(HaveOccurred())
			},
			wantLog: func(record string) {
				Expect(record).To(ContainSubstring(`"statusCode":403`))
				Expect(record).To(ContainSubstring(`"errorCode":"forbidden"`))
				Expect(record).To(ContainSubstring(`"serial":"serial-123"`))
			},
		}),
	)

	It("removes query strings and fragments from logged URIs", func() {
		// query には認証情報や検索条件が混ざる可能性があるため、ログには含めない。
		Expect(sanitizeAccessLogURI("https://example.com/path?token=secret#fragment")).To(Equal("https://example.com/path"))
	})
})

type accessLogCase struct {
	statusCode     int
	responseBody   string
	err            error
	wantStatusCode string
	wantMessage    string
	wantResult     func([]byte, error)
	wantLog        func(string)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
