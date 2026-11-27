package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

var domainLabel=regexp.MustCompile("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$")
func validDomain(d string)bool{
	if len(d)>253||!strings.Contains(d,"."){return false}
	for _,label:=range strings.Split(d,"."){if !domainLabel.MatchString(label){return false}}
	return true
}
func(s *Server) handleDomains(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodPost{writeError(w,405,"POST only");return}
	var req struct{Domains []string; ProxyGroup string; Exact bool}
	// Explicit wire names.
	var body struct{Domains []string `json:"domains"`; ProxyGroup string `json:"proxy_group"`; Exact bool `json:"exact"`}
	if err:=json.NewDecoder(r.Body).Decode(&body);err!=nil{writeError(w,400,"Invalid JSON");return}
	req.Domains,req.ProxyGroup,req.Exact=body.Domains,body.ProxyGroup,body.Exact
	if len(req.Domains)==0||len(req.Domains)>500||!slices.Contains(s.routingTargets(),req.ProxyGroup){writeError(w,400,"Choose a valid routing target and 1–500 domains");return}
	kind:="DOMAIN-SUFFIX";if req.Exact{kind="DOMAIN"}
	rules:=[]string{}
	for _,d:=range req.Domains{
		d=strings.ToLower(strings.TrimSpace(d))
		d=strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(d,"+."),"."),".")
		if !validDomain(d){writeError(w,400,fmt.Sprintf("Invalid domain: %q (use DNS name, not URL)",d));return}
		rule:=kind+","+d+","+req.ProxyGroup
		if !slices.Contains(rules,rule){rules=append(rules,rule)}
	}
	existing:=s.editor.GetRules()
	for _,rule:=range rules{
		if slices.Contains(existing,rule){continue}
		if err:=s.editor.AddRuleRaw(rule,-1);err!=nil{writeError(w,400,err.Error());return}
	}
	if err:=s.editor.Save();err!=nil{writeError(w,500,err.Error());return}
	writeJSON(w,200,SuccessResponse{Status:"ok",Message:"Domains saved and applied"})
}
