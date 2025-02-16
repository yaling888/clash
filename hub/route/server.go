package route

import (
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/gorilla/websocket"
	"github.com/phuslu/log"
	"go.uber.org/atomic"

	"github.com/yaling888/quirktiva/common/observable"
	"github.com/yaling888/quirktiva/common/pool"
	"github.com/yaling888/quirktiva/config"
	C "github.com/yaling888/quirktiva/constant"
	L "github.com/yaling888/quirktiva/log"
	"github.com/yaling888/quirktiva/tunnel/statistic"
)

var (
	serverSecret []byte
	serverAddr   = ""
	serverName   = ""

	uiPath = ""

	enablePPORF bool

	bootTime = atomic.NewTime(time.Now())

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func SetUIPath(path string) {
	uiPath = C.Path.Resolve(path)
}

func SetServerName(name string) {
	serverName = name
}

func SetPPROF(pprof bool) {
	enablePPORF = pprof
}

func Start(addr string, secret string) {
	if serverAddr != "" {
		return
	}

	serverAddr = addr
	serverSecret = []byte(secret)

	r := chi.NewRouter()

	corsM := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         300,
	})

	r.Use(corsM.Handler)
	r.Group(func(r chi.Router) {
		r.Use(authentication)

		r.Get("/", hello)
		r.Get("/logs", getLogs)
		r.Get("/traffic", traffic)
		r.Get("/version", version)
		r.Get("/uptime", uptime)
		r.Mount("/configs", configRouter())
		r.Mount("/configs/geo", configGeoRouter())
		r.Mount("/inbounds", inboundRouter())
		r.Mount("/proxies", proxyRouter())
		r.Mount("/rules", ruleRouter())
		r.Mount("/connections", connectionRouter())
		r.Mount("/providers/proxies", proxyProviderRouter())
		r.Mount("/cache", cacheRouter())
		r.Mount("/dns", dnsRouter())

		if enablePPORF {
			r.Mount("/debug/pprof", pprofRouter())
		}
		if serverName != "" {
			r.Mount("/dns-query", dohRouter())
		}
	})

	if uiPath != "" {
		r.Group(func(r chi.Router) {
			fs := http.StripPrefix("/ui", http.FileServer(http.Dir(uiPath)))
			r.Get("/ui", http.RedirectHandler("/ui/", http.StatusTemporaryRedirect).ServeHTTP)
			r.Get("/ui/*", func(w http.ResponseWriter, r *http.Request) {
				fs.ServeHTTP(w, r)
			})
		})
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error().Err(err).Msg("[API] external controller listen failed")
		return
	}
	serverAddr = l.Addr().String()

	e := log.Info().Str("addr", serverAddr)
	if serverName != "" {
		e.Str("serverName", serverName)
		certConfig, err := config.GetCertConfig()
		if err != nil {
			_ = l.Close()
			log.Error().Err(err).Msg("[API] get certificate config failed")
			return
		}
		var ips []net.IP
		if h, _, err := net.SplitHostPort(serverAddr); err == nil {
			if a, err := netip.ParseAddr(h); err == nil && a.IsGlobalUnicast() {
				ips = append(ips, a.AsSlice())
			}
		}
		ips = append(ips, netip.MustParseAddr("127.0.0.1").AsSlice(), netip.MustParseAddr("::1").AsSlice())
		certificate, err := certConfig.GetOrCreateCert(serverName, ips...)
		if err != nil {
			_ = l.Close()
			log.Error().Err(err).Msgf("[API] get certificate for server name: '%s' failed", serverName)
			return
		}
		tlsConfig := &tls.Config{
			NextProtos:   []string{"http/1.1", "h2"},
			Certificates: []tls.Certificate{*certificate},
		}
		l = tls.NewListener(l, tlsConfig)

		r.Get("/ca.crt", func(w http.ResponseWriter, r *http.Request) {
			b := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: certConfig.GetRootCA().Raw,
			})
			w.Header().Set("Content-Type", "application/x-x509-ca-cert")
			_, _ = w.Write(b)
		})
	}
	e.Msg("[API] listening")
	if err = http.Serve(l, r); err != nil {
		log.Error().Err(err).Msg("[API] external controller serve failed")
	}
}

func authentication(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if len(serverSecret) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Browser websocket not support custom header
		if websocket.IsWebSocketUpgrade(r) && r.URL.Query().Get("token") != "" {
			token := r.URL.Query().Get("token")
			if subtle.ConstantTimeCompare([]byte(token), serverSecret) != 1 {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		bearer, token, found := strings.Cut(header, " ")
		if header == "" {
			if token = r.URL.Query().Get("token"); token != "" {
				found = true
			}
		}

		hasInvalidHeader := header != "" && bearer != "Bearer"
		hasInvalidSecret := !found || subtle.ConstantTimeCompare([]byte(token), serverSecret) != 1
		if hasInvalidHeader || hasInvalidSecret {
			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func hello(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, render.M{"hello": "clash plus pro"})
}

func traffic(w http.ResponseWriter, r *http.Request) {
	var wsConn *websocket.Conn
	if websocket.IsWebSocketUpgrade(r) {
		var err error
		wsConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
	}

	if wsConn == nil {
		w.Header().Set("Content-Type", "application/json")
		render.Status(r, http.StatusOK)
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	t := statistic.DefaultManager
	buf := pool.BufferWriter{}
	encoder := json.NewEncoder(&buf)
	var err error
	for range tick.C {
		buf.Reset()
		up, down := t.Now()
		if err := encoder.Encode(Traffic{
			Up:   up,
			Down: down,
		}); err != nil {
			break
		}

		if wsConn == nil {
			_, err = w.Write(buf.Bytes())
			w.(http.Flusher).Flush()
		} else {
			err = wsConn.WriteMessage(websocket.TextMessage, buf.Bytes())
		}

		if err != nil {
			break
		}
	}
}

type Log struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func getLogs(w http.ResponseWriter, r *http.Request) {
	var (
		levelText = r.URL.Query().Get("level")
		format    = r.URL.Query().Get("format")
	)
	if levelText == "" {
		levelText = "info"
	}
	if format == "" {
		format = "text"
	}

	level, ok := L.LogLevelMapping[levelText]
	if !ok {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	var wsConn *websocket.Conn
	if websocket.IsWebSocketUpgrade(r) {
		var err error
		wsConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
	}

	var (
		sub     observable.Subscription[L.Event]
		ch      = make(chan L.Event, 1024)
		buf     = pool.BufferWriter{}
		encoder = json.NewEncoder(&buf)
		closed  = false
	)

	if wsConn == nil {
		w.Header().Set("Content-Type", "application/json")
		render.Status(r, http.StatusOK)
	} else if level > L.INFO {
		go func() {
			for _, _, err := wsConn.ReadMessage(); err != nil; {
				closed = true
				break
			}
		}()
	}

	if strings.EqualFold(format, "structured") {
		sub = L.SubscribeJson()
		defer L.UnSubscribeJson(sub)
	} else {
		sub = L.SubscribeText()
		defer L.UnSubscribeText(sub)
	}

	go func() {
		for elm := range sub {
			select {
			case ch <- elm:
			default:
			}
		}
		close(ch)
	}()

	for logM := range ch {
		if closed {
			break
		}
		if logM.LogLevel < level {
			continue
		}
		buf.Reset()

		if err := encoder.Encode(Log{
			Type:    logM.Type(),
			Payload: logM.Payload,
		}); err != nil {
			break
		}

		var err error
		if wsConn == nil {
			_, err = w.Write(buf.Bytes())
			w.(http.Flusher).Flush()
		} else {
			err = wsConn.WriteMessage(websocket.TextMessage, buf.Bytes())
		}

		if err != nil {
			break
		}
	}
}

func version(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, render.M{"version": "PlusPro-" + C.Version, "plus-pro": true})
}

func uptime(w http.ResponseWriter, r *http.Request) {
	bt := bootTime.Load()
	render.JSON(w, r, render.M{
		"bootTime": bt.Format("2006-01-02 15:04:05 Mon -0700"),
	})
}
