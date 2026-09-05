// Package decoy — «under construction» сайт на :443.
//
// Отдаётся всем, кто пришёл на узел без валидного токена (см. auth): снаружи узел
// выглядит как обычный HTTPS-сайт-заглушка. Тот же HTTP-слой переиспользуется
// клиентской частью для локальной веб-панели настроек и (по admin-токену)
// панели управления узлом.
package decoy

import "net/http"

const page = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Under construction</title>
<style>
 body{font-family:system-ui,sans-serif;background:#0e0f12;color:#c9ccd3;
      display:grid;place-items:center;height:100vh;margin:0}
 main{text-align:center;max-width:32rem;padding:2rem}
 h1{font-weight:600;letter-spacing:.02em}
 p{color:#8a8f99}
</style></head><body><main>
 <h1>Under construction</h1>
 <p>This site is being set up. Please check back later.</p>
</main></body></html>`

// Handler возвращает http.Handler decoy-страницы.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(page))
	})
}
