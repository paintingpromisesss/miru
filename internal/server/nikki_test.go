package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"miru/internal/config"
)
func TestNikkiEditAndRollback(t *testing.T){
	dir:=t.TempDir();p:=filepath.Join(dir,"mixin.yaml");eff:=filepath.Join(dir,"runtime.yaml");rd:=filepath.Join(dir,"ruleset")
	os.Mkdir(rd,0700)
	original:="nikki-proxy-groups:\n  - name: VPN-NON-RU\n    type: url-test\n    include-all-proxies: true\nnikki-rules:\n  - MATCH,DIRECT\n"
	os.WriteFile(p,[]byte(original),0600)
	os.WriteFile(eff,[]byte("proxies:\n  - name: node\n    type: direct\n"),0600)
	e,err:=config.NewModeEditor(p,"nikki");if err!=nil{t.Fatal(err)}
	fail:=false;calls:=0
	s:=New(Config{Editor:e,ConfigPath:p,RulesDir:rd,Mode:"nikki",EffectivePath:eff,Apply:func()error{calls++;if fail&&calls==2{return fmt.Errorf("bad apply")};return nil}})
	mux:=http.NewServeMux();s.RegisterRoutes(mux)
	send:=func(name string)*httptest.ResponseRecorder{
		r:=httptest.NewRequest("POST","/api/rules",strings.NewReader(fmt.Sprintf("{\"name\":%q,\"url\":\"https://example.org/rules.mrs\",\"behavior\":\"domain\",\"format\":\"mrs\",\"proxy_group\":\"VPN-NON-RU\"}",name)))
		w:=httptest.NewRecorder();mux.ServeHTTP(w,r);return w
	}
	if w:=send("spotify");w.Code!=200{t.Fatal(w.Code,w.Body.String())}
	b,_:=os.ReadFile(p)
	if !strings.Contains(string(b),"RULE-SET,spotify,VPN-NON-RU")||!strings.Contains(string(b),"./ruleset/spotify.mrs")||strings.Contains(string(b),"\nrules:"){t.Fatal(string(b))}
	if !strings.Contains(strings.Join(s.routingTargets(),","),"node"){t.Fatal("missing inherited node")}
	fail=true
	if w:=send("broken");w.Code!=502{t.Fatal(w.Code,w.Body.String())}
	restored,_:=os.ReadFile(p);if string(restored)!=string(b){t.Fatal("rollback did not restore exact file")}
}

func TestNikkiCustomDomains(t *testing.T){
	p:=filepath.Join(t.TempDir(),"mixin.yaml")
	os.WriteFile(p,[]byte("nikki-proxy-groups:\n  - name: VPN-NON-RU\n    type: select\nnikki-rules:\n  - MATCH,DIRECT\n"),0600)
	e,err:=config.NewModeEditor(p,"nikki");if err!=nil{t.Fatal(err)}
	s:=New(Config{Editor:e,Mode:"nikki",ConfigPath:p,Apply:func()error{return nil}})
	m:=http.NewServeMux();s.RegisterRoutes(m)
	r:=httptest.NewRequest("POST","/api/domains",strings.NewReader("{\"domains\":[\"example.org\",\"+.Example.com\",\"example.org\"],\"proxy_group\":\"VPN-NON-RU\"}"))
	w:=httptest.NewRecorder();m.ServeHTTP(w,r)
	if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
	b,_:=os.ReadFile(p);str:=string(b)
	if strings.Count(str,"DOMAIN-SUFFIX,example.org,VPN-NON-RU")!=1||!strings.Contains(str,"DOMAIN-SUFFIX,example.com,VPN-NON-RU")||strings.Index(str,"DOMAIN-SUFFIX")>strings.Index(str,"MATCH,DIRECT"){t.Fatal(str)}
}
