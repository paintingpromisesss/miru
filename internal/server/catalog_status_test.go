package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miru/internal/catalog"
	"miru/internal/config"
)

func TestCatalogActiveRequiresRoutingAndCorrectProvider(t *testing.T) {
	s,mux,_,dir:=setupTestServer(t)
	const url="https://raw.githubusercontent.com/paintingpromisesss/tachyon-mihomo/generated/spotify-domain.mrs"
	s.catalog.SetFilesForTest([]catalog.GitHubFile{
		{Name:"spotify-domain",Path:"spotify-domain.mrs",Behavior:"domain",URL:url},
		{Name:"spotify-other",Path:"spotify-domain.mrs",Behavior:"domain",URL:"https://example.org/spotify-domain.mrs"},
	})
	if err:=s.editor.AddRuleProvider(config.RuleProviderEntry{Name:"spotify-domain",URL:url,Behavior:"domain",Format:"mrs"});err!=nil{t.Fatal(err)}
	if err:=s.editor.Save();err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(dir,"spotify-domain.mrs"),[]byte("cache"),0600);err!=nil{t.Fatal(err)}
	get:=func()[]CatalogItemResponse{
		w:=httptest.NewRecorder();mux.ServeHTTP(w,httptest.NewRequest("GET","/api/catalog",nil))
		var v struct{Items []CatalogItemResponse}
		if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
		if err:=json.Unmarshal(w.Body.Bytes(),&v);err!=nil{t.Fatal(err)}
		return v.Items
	}
	v:=get()
	if v[0].Applied||!v[0].Registered||!v[0].Downloaded||v[1].Applied||v[1].Registered{t.Fatalf("incorrect initial status: %+v",v)}
	w:=httptest.NewRecorder()
	mux.ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/api/rules",strings.NewReader("{\"name\":\"spotify-domain\",\"url\":\""+url+"\",\"proxy_group\":\"UNBLOCK\",\"behavior\":\"domain\",\"format\":\"mrs\"}")))
	if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
	v=get()
	if !v[0].Applied||v[1].Applied{t.Fatalf("incorrect applied status: %+v",v)}
	w=httptest.NewRecorder()
	mux.ServeHTTP(w,httptest.NewRequest(http.MethodDelete,"/api/rules?name=spotify-domain&delete_file=false",nil))
	if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
	v=get()
	if v[0].Applied||v[0].Registered||!v[0].Downloaded{t.Fatalf("cached file must not imply active: %+v",v)}
}
