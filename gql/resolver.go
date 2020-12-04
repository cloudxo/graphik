package gql

import (
	"context"
	"encoding/gob"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/autom8ter/graphik/gen/go"
	"github.com/autom8ter/graphik/gen/gql/generated"
	"github.com/autom8ter/graphik/generic/cache"
	"github.com/autom8ter/graphik/helpers"
	"github.com/autom8ter/graphik/logger"
	"github.com/autom8ter/machine"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/metadata"
	"html/template"
	"net/http"
	"time"
)

func init() {
	gob.Register(&oauth2.Token{})
}

// This file will not be regenerated automatically.
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	client     apipb.DatabaseServiceClient
	cors       *cors.Cors
	machine    *machine.Machine
	store      *cache.Cache
	config     *oauth2.Config
	cookieName string
}

func NewResolver(ctx context.Context, client apipb.DatabaseServiceClient, cors *cors.Cors, config *oauth2.Config, cache *cache.Cache) *Resolver {
	return &Resolver{
		client:     client,
		cors:       cors,
		machine:    machine.New(ctx),
		config:     config,
		store:      cache,
		cookieName: "graphik",
	}
}

func (r *Resolver) QueryHandler() http.Handler {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{
		Resolvers:  r,
		Directives: generated.DirectiveRoot{},
		Complexity: generated.ComplexityRoot{},
	}))
	srv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, error) {
			auth := initPayload.Authorization()
			ctx = metadata.AppendToOutgoingContext(ctx, "Authorization", auth)
			return ctx, nil
		},
		KeepAlivePingInterval: 10 * time.Second,
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.SetQueryCache(lru.New(1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New(100),
	})
	return r.cors.Handler(r.authMiddleware(srv))
}

func (r *Resolver) authMiddleware(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if r.store != nil {
			cookie, _ := req.Cookie("graphik")
			if cookie != nil && cookie.Value != "" && req.Header.Get("Authorization") == "" {
				val, ok := r.store.Get(cookie.Value)
				if ok {
					token, ok := val.(*oauth2.Token)
					if ok && token.Expiry.After(time.Now()) {
						req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
					}
				}
			}
		}
		for k, arr := range req.Header {
			if len(arr) > 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, k, arr[0])
			}
		}
		handler.ServeHTTP(w, req.WithContext(ctx))
	}
}

func (r *Resolver) Playground() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.config == nil || r.config.ClientID == "" {
			http.Error(w, "playground disabled", http.StatusNotFound)
			return
		}
		cookie, err := req.Cookie(r.cookieName)
		if err != nil {
			logger.Info("unauthenticed - redirecting to login")
			r.redirectLogin(w, req)
			return
		}
		if cookie.Value == "" {
			r.redirectLogin(w, req)
			return
		}
		val, ok := r.store.Get(cookie.Value)
		if !ok {
			r.redirectLogin(w, req)
			return
		}
		authToken, ok := val.(*oauth2.Token)
		if !ok {
			r.redirectLogin(w, req)
			return
		}
		if authToken == nil {
			r.redirectLogin(w, req)
			return
		}
		if authToken.Expiry.Before(time.Now()) {
			r.redirectLogin(w, req)
			return
		}
		w.Header().Add("Content-Type", "text/html")
		var playground = template.Must(template.New("playground").Parse(`<!DOCTYPE html>
<html>

<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, minimum-scale=1.0, maximum-scale=1.0, minimal-ui">
  <title>Graphik Playground</title>
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>

<body>
  <div id="root">
    <style>
      body {
        background-color: rgb(23, 42, 58);
        font-family: Open Sans, sans-serif;
        height: 90vh;
      }

      #root {
        height: 100%;
        width: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
      }

      .loading {
        font-size: 32px;
        font-weight: 200;
        color: rgba(255, 255, 255, .6);
        margin-left: 20px;
      }

      img {
        width: 78px;
        height: 78px;
      }

      .title {
        font-weight: 400;
      }
    </style>
    <img src='//cdn.jsdelivr.net/npm/graphql-playground-react/build/logo.png' alt=''>
    <div class="loading"> Loading
      <span class="title">Graphik Playground</span>
    </div>
  </div>
  <script>window.addEventListener('load', function (event) {
 		const wsProto = location.protocol == 'https:' ? 'wss:' : 'ws:'
      GraphQLPlayground.init(document.getElementById('root'), {
		endpoint: location.protocol + '//' + location.host,
		subscriptionsEndpoint: wsProto + '//' + location.host,
		shareEnabled: true,
		headers: {
			'Authorization': 'Bearer {{.token }}'
		},
		settings: {
			'request.credentials': 'same-origin'
		}
      })
    })</script>
</body>

</html>
`))

		playground.Execute(w, map[string]string{
			"token": authToken.AccessToken,
		})
	}
}

func (r *Resolver) redirectLogin(w http.ResponseWriter, req *http.Request) {
	state := helpers.Hash([]byte(ksuid.New().String()))
	redirect := r.config.AuthCodeURL(state)
	http.Redirect(w, req, redirect, http.StatusTemporaryRedirect)
}

func (r *Resolver) PlaygroundCallback(playgroundRedirect string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.config == nil || r.config.ClientID == "" {
			http.Error(w, "playground disabled", http.StatusNotFound)
			return
		}
		code := req.URL.Query().Get("code")
		state := req.URL.Query().Get("state")
		if code == "" {
			http.Error(w, "empty authorization code", http.StatusBadRequest)
			return
		}
		if state == "" {
			http.Error(w, "empty authorization state", http.StatusBadRequest)
			return
		}
		//stateVal := sess.Values["state"]
		//if stateVal == nil {
		//	http.Error(w, "failed to get session state", http.StatusForbidden)
		//	return
		//}
		//if stateVal.(string) != state {
		//	http.Error(w, fmt.Sprintf("session state mismatch: %s", stateVal.(string)), http.StatusForbidden)
		//	return
		//}
		token, err := r.config.Exchange(req.Context(), code)
		if err != nil {
			logger.Error("failed to exchange authorization code", zap.Error(err))
			http.Error(w, "failed to exchange authorization code", http.StatusInternalServerError)
			return
		}
		id := helpers.Hash([]byte(ksuid.New().String()))
		http.SetCookie(w, &http.Cookie{
			Name:    r.cookieName,
			Value:   id,
			Expires: time.Now().Add(24 * time.Hour),
		})
		r.store.Set(id, token, 1*time.Hour)
		http.Redirect(w, req, playgroundRedirect, http.StatusTemporaryRedirect)
	}
}
