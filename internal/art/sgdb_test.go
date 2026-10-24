package art

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSteamGridDB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			w.Write([]byte(`{"success":false,"errors":["Authentication Required"]}`))
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/autocomplete/"):
			w.Write([]byte(`{"success":true,"data":[{"id":42,"name":"Mario Strikers Charged","release_date":1179446400}]}`))
		case strings.HasPrefix(r.URL.Path, "/heroes/game/42"):
			w.Write([]byte(`{"success":true,"data":[{"url":"https://cdn2.steamgriddb.com/hero/a.png","thumb":"https://cdn2.steamgriddb.com/hero_thumb/a.jpg","width":1920,"height":620,"author":{"name":"someone"}}]}`))
		case strings.HasPrefix(r.URL.Path, "/logos/game/42"):
			w.Write([]byte(`{"success":true,"data":[{"url":"https://cdn2.steamgriddb.com/logo/b.png","thumb":"https://cdn2.steamgriddb.com/logo_thumb/b.png","width":800,"height":300}]}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"success":false,"errors":["not found"]}`))
		}
	}))
	defer srv.Close()
	old := sgdbAPI
	sgdbAPI = srv.URL + "/"
	defer func() { sgdbAPI = old }()
	ctx := context.Background()
	if err := CheckKey(ctx, "bad"); err == nil || !strings.Contains(err.Error(), "did not accept") {
		t.Fatalf("bad key: %v", err)
	}
	if err := CheckKey(ctx, ""); err == nil {
		t.Fatal("empty key accepted")
	}
	g, err := SteamGridGames(ctx, "good", "mario strikers")
	if err != nil || len(g) != 1 || g[0].ID != 42 || g[0].Year != 2007 {
		t.Fatalf("%v %+v", err, g)
	}
	c, err := SteamGridImages(ctx, "good", 42, "Mario Strikers Charged")
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]int{}
	for _, x := range c {
		slots[x.Slot]++
	}
	if slots["background"] != 1 || slots["logo"] != 1 || c[0].Credit == "" && c[1].Credit == "" {
		t.Fatalf("%+v", c)
	}
}
