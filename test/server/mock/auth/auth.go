package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"time"
)

func Mock(config configuration.Config, ctx context.Context) (err error) {
	router, err := getRouter()
	if err != nil {
		return err
	}
	server := httptest.NewServer(router)
	config.AuthEndpoint = server.URL
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	return nil
}

func getRouter() (router *httprouter.Router, err error) {
	defer func() {
		if r := recover(); r != nil && err == nil {
			log.Printf("%s: %s", r, debug.Stack())
			err = errors.New(fmt.Sprint("Recovered Error: ", r))
		}
	}()
	router = httprouter.New()

	router.POST("/auth/realms/master/protocol/openid-connect/token", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		username := request.FormValue("username")
		var token string
		if request.FormValue("grant_type") == "password" && username == "" {
			http.Error(writer, "missing user name", 400)
			return
		}
		if request.FormValue("grant_type") == "urn:ietf:params:oauth:grant-type:token-exchange" {
			username = request.FormValue("requested_subject")
		}
		if username == "admin" {
			token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJiOGUyNGZkNy1jNjJlLTRhNWQtOTQ4ZC1mZGI2ZWVkM2JmYzYiLCJleHAiOjE1MzA1MzIwMzIsIm5iZiI6MCwiaWF0IjoxNTMwNTI4NDMyLCJpc3MiOiJodHRwczovL2F1dGguc2VwbC5pbmZhaS5vcmcvYXV0aC9yZWFsbXMvbWFzdGVyIiwiYXVkIjoiZnJvbnRlbmQiLCJzdWIiOiJhZG1pbiIsInR5cCI6IkJlYXJlciIsImF6cCI6ImZyb250ZW5kIiwibm9uY2UiOiIyMmUwZWNmOC1mOGExLTQ0NDUtYWYyNy00ZDUzYmY1ZDE4YjkiLCJhdXRoX3RpbWUiOjE1MzA1Mjg0MjMsInNlc3Npb25fc3RhdGUiOiIxZDc1YTk4NC03MzU5LTQxYmUtODFiOS03MzJkODI3NGMyM2UiLCJhY3IiOiIwIiwiYWxsb3dlZC1vcmlnaW5zIjpbIioiXSwicmVhbG1fYWNjZXNzIjp7InJvbGVzIjpbImNyZWF0ZS1yZWFsbSIsImFkbWluIiwiZGV2ZWxvcGVyIiwidW1hX2F1dGhvcml6YXRpb24iLCJ1c2VyIl19LCJyZXNvdXJjZV9hY2Nlc3MiOnsibWFzdGVyLXJlYWxtIjp7InJvbGVzIjpbInZpZXctaWRlbnRpdHktcHJvdmlkZXJzIiwidmlldy1yZWFsbSIsIm1hbmFnZS1pZGVudGl0eS1wcm92aWRlcnMiLCJpbXBlcnNvbmF0aW9uIiwiY3JlYXRlLWNsaWVudCIsIm1hbmFnZS11c2VycyIsInF1ZXJ5LXJlYWxtcyIsInZpZXctYXV0aG9yaXphdGlvbiIsInF1ZXJ5LWNsaWVudHMiLCJxdWVyeS11c2VycyIsIm1hbmFnZS1ldmVudHMiLCJtYW5hZ2UtcmVhbG0iLCJ2aWV3LWV2ZW50cyIsInZpZXctdXNlcnMiLCJ2aWV3LWNsaWVudHMiLCJtYW5hZ2UtYXV0aG9yaXphdGlvbiIsIm1hbmFnZS1jbGllbnRzIiwicXVlcnktZ3JvdXBzIl19LCJhY2NvdW50Ijp7InJvbGVzIjpbIm1hbmFnZS1hY2NvdW50IiwibWFuYWdlLWFjY291bnQtbGlua3MiLCJ2aWV3LXByb2ZpbGUiXX19LCJyb2xlcyI6WyJ1bWFfYXV0aG9yaXphdGlvbiIsImFkbWluIiwiY3JlYXRlLXJlYWxtIiwiZGV2ZWxvcGVyIiwidXNlciIsIm9mZmxpbmVfYWNjZXNzIl0sIm5hbWUiOiJkZiBkZmZmZiIsInByZWZlcnJlZF91c2VybmFtZSI6InNlcGwiLCJnaXZlbl9uYW1lIjoiZGYiLCJmYW1pbHlfbmFtZSI6ImRmZmZmIiwiZW1haWwiOiJzZXBsQHNlcGwuZGUifQ.kefK08rDs_PR4Ayzgh4nbK9SDKbNbQLZgH-NyPvysU4"
		} else if username == "user" {
			token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJiOGUyNGZkNy1jNjJlLTRhNWQtOTQ4ZC1mZGI2ZWVkM2JmYzYiLCJleHAiOjE1MzA1MzIwMzIsIm5iZiI6MCwiaWF0IjoxNTMwNTI4NDMyLCJpc3MiOiJodHRwczovL2F1dGguc2VwbC5pbmZhaS5vcmcvYXV0aC9yZWFsbXMvbWFzdGVyIiwiYXVkIjoiZnJvbnRlbmQiLCJzdWIiOiJ1c2VyIiwidHlwIjoiQmVhcmVyIiwiYXpwIjoiZnJvbnRlbmQiLCJub25jZSI6IjIyZTBlY2Y4LWY4YTEtNDQ0NS1hZjI3LTRkNTNiZjVkMThiOSIsImF1dGhfdGltZSI6MTUzMDUyODQyMywic2Vzc2lvbl9zdGF0ZSI6IjFkNzVhOTg0LTczNTktNDFiZS04MWI5LTczMmQ4Mjc0YzIzZSIsImFjciI6IjAiLCJhbGxvd2VkLW9yaWdpbnMiOlsiKiJdLCJyZWFsbV9hY2Nlc3MiOnsicm9sZXMiOlsiY3JlYXRlLXJlYWxtIiwiZGV2ZWxvcGVyIiwidW1hX2F1dGhvcml6YXRpb24iLCJ1c2VyIl19LCJyZXNvdXJjZV9hY2Nlc3MiOnsibWFzdGVyLXJlYWxtIjp7InJvbGVzIjpbInZpZXctaWRlbnRpdHktcHJvdmlkZXJzIiwidmlldy1yZWFsbSIsIm1hbmFnZS1pZGVudGl0eS1wcm92aWRlcnMiLCJpbXBlcnNvbmF0aW9uIiwiY3JlYXRlLWNsaWVudCIsIm1hbmFnZS11c2VycyIsInF1ZXJ5LXJlYWxtcyIsInZpZXctYXV0aG9yaXphdGlvbiIsInF1ZXJ5LWNsaWVudHMiLCJxdWVyeS11c2VycyIsIm1hbmFnZS1ldmVudHMiLCJtYW5hZ2UtcmVhbG0iLCJ2aWV3LWV2ZW50cyIsInZpZXctdXNlcnMiLCJ2aWV3LWNsaWVudHMiLCJtYW5hZ2UtYXV0aG9yaXphdGlvbiIsIm1hbmFnZS1jbGllbnRzIiwicXVlcnktZ3JvdXBzIl19LCJhY2NvdW50Ijp7InJvbGVzIjpbIm1hbmFnZS1hY2NvdW50IiwibWFuYWdlLWFjY291bnQtbGlua3MiLCJ2aWV3LXByb2ZpbGUiXX19LCJyb2xlcyI6WyJ1bWFfYXV0aG9yaXphdGlvbiIsImNyZWF0ZS1yZWFsbSIsImRldmVsb3BlciIsInVzZXIiLCJvZmZsaW5lX2FjY2VzcyJdLCJuYW1lIjoidXNlciIsInByZWZlcnJlZF91c2VybmFtZSI6InNlcGwiLCJnaXZlbl9uYW1lIjoiZGYiLCJmYW1pbHlfbmFtZSI6ImRmZmZmIiwiZW1haWwiOiJzZXBsQHNlcGwuZGUifQ.ieB5XGFQkvCrd3mINQoiSCrIEylz5GVoe-y7HiPDj9c"
		} else if username == "sepl" {
			token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJiOGUyNGZkNy1jNjJlLTRhNWQtOTQ4ZC1mZGI2ZWVkM2JmYzYiLCJleHAiOjE1MzA1MzIwMzIsIm5iZiI6MCwiaWF0IjoxNTMwNTI4NDMyLCJpc3MiOiJodHRwczovL2F1dGguc2VwbC5pbmZhaS5vcmcvYXV0aC9yZWFsbXMvbWFzdGVyIiwiYXVkIjoiZnJvbnRlbmQiLCJzdWIiOiJzZXBsIiwidHlwIjoiQmVhcmVyIiwiYXpwIjoiZnJvbnRlbmQiLCJub25jZSI6IjIyZTBlY2Y4LWY4YTEtNDQ0NS1hZjI3LTRkNTNiZjVkMThiOSIsImF1dGhfdGltZSI6MTUzMDUyODQyMywic2Vzc2lvbl9zdGF0ZSI6IjFkNzVhOTg0LTczNTktNDFiZS04MWI5LTczMmQ4Mjc0YzIzZSIsImFjciI6IjAiLCJhbGxvd2VkLW9yaWdpbnMiOlsiKiJdLCJyZWFsbV9hY2Nlc3MiOnsicm9sZXMiOlsiY3JlYXRlLXJlYWxtIiwiYWRtaW4iLCJkZXZlbG9wZXIiLCJ1bWFfYXV0aG9yaXphdGlvbiIsInVzZXIiXX0sInJlc291cmNlX2FjY2VzcyI6eyJtYXN0ZXItcmVhbG0iOnsicm9sZXMiOlsidmlldy1pZGVudGl0eS1wcm92aWRlcnMiLCJ2aWV3LXJlYWxtIiwibWFuYWdlLWlkZW50aXR5LXByb3ZpZGVycyIsImltcGVyc29uYXRpb24iLCJjcmVhdGUtY2xpZW50IiwibWFuYWdlLXVzZXJzIiwicXVlcnktcmVhbG1zIiwidmlldy1hdXRob3JpemF0aW9uIiwicXVlcnktY2xpZW50cyIsInF1ZXJ5LXVzZXJzIiwibWFuYWdlLWV2ZW50cyIsIm1hbmFnZS1yZWFsbSIsInZpZXctZXZlbnRzIiwidmlldy11c2VycyIsInZpZXctY2xpZW50cyIsIm1hbmFnZS1hdXRob3JpemF0aW9uIiwibWFuYWdlLWNsaWVudHMiLCJxdWVyeS1ncm91cHMiXX0sImFjY291bnQiOnsicm9sZXMiOlsibWFuYWdlLWFjY291bnQiLCJtYW5hZ2UtYWNjb3VudC1saW5rcyIsInZpZXctcHJvZmlsZSJdfX0sInJvbGVzIjpbInVtYV9hdXRob3JpemF0aW9uIiwiYWRtaW4iLCJjcmVhdGUtcmVhbG0iLCJkZXZlbG9wZXIiLCJ1c2VyIiwib2ZmbGluZV9hY2Nlc3MiXSwibmFtZSI6ImRmIGRmZmZmIiwicHJlZmVycmVkX3VzZXJuYW1lIjoic2VwbCIsImdpdmVuX25hbWUiOiJkZiIsImZhbWlseV9uYW1lIjoiZGZmZmYiLCJlbWFpbCI6InNlcGxAc2VwbC5kZSJ9.ADj6QJ2JglCLcdOYhHPGyjeN-h8HhmJDEEO0861NgrU"
		} else {
			log.Println("DEBUG: auth/auth.go::getRouter username=", username)
			token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJiOGUyNGZkNy1jNjJlLTRhNWQtOTQ4ZC1mZGI2ZWVkM2JmYzYiLCJleHAiOjE1MzA1MzIwMzIsIm5iZiI6MCwiaWF0IjoxNTMwNTI4NDMyLCJpc3MiOiJodHRwczovL2F1dGguc2VwbC5pbmZhaS5vcmcvYXV0aC9yZWFsbXMvbWFzdGVyIiwiYXVkIjoiZnJvbnRlbmQiLCJzdWIiOiJ1bmtub3duIiwidHlwIjoiQmVhcmVyIiwiYXpwIjoiZnJvbnRlbmQiLCJub25jZSI6IjIyZTBlY2Y4LWY4YTEtNDQ0NS1hZjI3LTRkNTNiZjVkMThiOSIsImF1dGhfdGltZSI6MTUzMDUyODQyMywic2Vzc2lvbl9zdGF0ZSI6IjFkNzVhOTg0LTczNTktNDFiZS04MWI5LTczMmQ4Mjc0YzIzZSIsImFjciI6IjAiLCJhbGxvd2VkLW9yaWdpbnMiOlsiKiJdLCJyZWFsbV9hY2Nlc3MiOnsicm9sZXMiOlsiY3JlYXRlLXJlYWxtIiwiYWRtaW4iLCJkZXZlbG9wZXIiLCJ1bWFfYXV0aG9yaXphdGlvbiIsInVzZXIiXX0sInJlc291cmNlX2FjY2VzcyI6eyJtYXN0ZXItcmVhbG0iOnsicm9sZXMiOlsidmlldy1pZGVudGl0eS1wcm92aWRlcnMiLCJ2aWV3LXJlYWxtIiwibWFuYWdlLWlkZW50aXR5LXByb3ZpZGVycyIsImltcGVyc29uYXRpb24iLCJjcmVhdGUtY2xpZW50IiwibWFuYWdlLXVzZXJzIiwicXVlcnktcmVhbG1zIiwidmlldy1hdXRob3JpemF0aW9uIiwicXVlcnktY2xpZW50cyIsInF1ZXJ5LXVzZXJzIiwibWFuYWdlLWV2ZW50cyIsIm1hbmFnZS1yZWFsbSIsInZpZXctZXZlbnRzIiwidmlldy11c2VycyIsInZpZXctY2xpZW50cyIsIm1hbmFnZS1hdXRob3JpemF0aW9uIiwibWFuYWdlLWNsaWVudHMiLCJxdWVyeS1ncm91cHMiXX0sImFjY291bnQiOnsicm9sZXMiOlsibWFuYWdlLWFjY291bnQiLCJtYW5hZ2UtYWNjb3VudC1saW5rcyIsInZpZXctcHJvZmlsZSJdfX0sInJvbGVzIjpbInVtYV9hdXRob3JpemF0aW9uIiwiYWRtaW4iLCJjcmVhdGUtcmVhbG0iLCJkZXZlbG9wZXIiLCJ1c2VyIiwib2ZmbGluZV9hY2Nlc3MiXSwibmFtZSI6ImRmIGRmZmZmIiwicHJlZmVycmVkX3VzZXJuYW1lIjoic2VwbCIsImdpdmVuX25hbWUiOiJkZiIsImZhbWlseV9uYW1lIjoiZGZmZmYiLCJlbWFpbCI6InNlcGxAc2VwbC5kZSJ9._n-uZVXMbezJ8nN2UTbOFPQgl-BuxKnTwkZtBIxMYOs"
		}
		err = json.NewEncoder(writer).Encode(security.OpenidToken{
			AccessToken:      token,
			ExpiresIn:        10 * float64(time.Hour),
			RefreshExpiresIn: 5 * float64(time.Hour),
			RefreshToken:     "",
			TokenType:        "",
			RequestTime:      time.Now(),
		})
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
	})

	router.GET("/auth/admin/realms/master/users", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		username := request.URL.Query().Get("username")
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		err = json.NewEncoder(writer).Encode([]security.UserRepresentation{{
			Id:   username,
			Name: username,
		}})
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
	})

	router.GET("/auth/admin/realms/master/users/:userid/role-mappings/realm", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		if params.ByName("userid") == "admin" {
			err = json.NewEncoder(writer).Encode([]security.RoleMapping{{Name: "admin"}})
		} else {
			err = json.NewEncoder(writer).Encode([]security.RoleMapping{{Name: "test-group"}})
		}
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
	})

	return
}
