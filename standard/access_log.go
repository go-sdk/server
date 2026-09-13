package standard

import (
	"net/http"
	"time"

	"github.com/go-sdk/core/logx"
)

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.size += n
	return n, err
}

func httpAccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		writer := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		statusCode := writer.status
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		logx.Ctx(r.Context()).Info().
			Str("http_method", r.Method).
			Str("http_path", r.URL.Path).
			Int("status_code", statusCode).
			Int("content_length", writer.size).
			Str("client_ip", remoteIP(r.RemoteAddr)).
			Str("content_type", r.Header.Get("Content-Type")).
			Str("user_agent", r.UserAgent()).
			Dur("duration", time.Since(startedAt).Truncate(time.Millisecond)).
			Msg("http request completed")
	})
}
