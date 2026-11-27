package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"miru/internal/config"
)

func(s *Server) routingTargets() []string {
	targets:=s.editor.GetDiscoveredProxies()
	if s.mode=="nikki" && s.effectivePath!="" {
		if effective,err:=config.NewYAMLEditor(s.effectivePath);err==nil{
			for _,v:=range effective.GetDiscoveredProxies(){if !slices.Contains(targets,v){targets=append(targets,v)}}
		}
	}
	return targets
}

// Serialize edits and reload, and do not return a success response before
// applying the persisted config. Reads see the latest source, not stale AST.
func(s *Server) guard(next http.HandlerFunc)http.HandlerFunc{
	return func(w http.ResponseWriter,r *http.Request){
		s.txMu.Lock();defer s.txMu.Unlock()
		if r.Method==http.MethodGet || r.Method==http.MethodHead {
			if s.editor!=nil {if err:=s.editor.ReloadFromDisk();err!=nil{writeError(w,500,"Read config: "+err.Error());return}}
			next(w,r);return
		}
		if r.URL.Path=="/api/reload"{next(w,r);return}
		if s.editor==nil{next(w,r);return}
		r.Body=http.MaxBytesReader(w,r.Body,1<<20)
		before,err:=os.ReadFile(s.configPath)
		if err!=nil{writeError(w,500,err.Error());return}
		if err=s.editor.ReloadFromDisk();err!=nil{writeError(w,500,err.Error());return}
		s.pendingDeletes=nil
		defer func(){s.pendingDeletes=nil}()
		rec:=httptest.NewRecorder()
		next(rec,r)
		after,readErr:=os.ReadFile(s.configPath)
		if rec.Code>=400 || readErr!=nil {
			if err=s.editor.Restore(before);err!=nil{writeError(w,500,"Restore failed: "+err.Error());return}
			if readErr!=nil{writeError(w,500,readErr.Error());return}
		}else if !bytes.Equal(before,after){
			if err=config.AtomicBytes(s.configPath+".miru.bak",before);err==nil{err=s.ReloadMihomo()}
			if err!=nil{
				applyErr:=err
				if restoreErr:=s.editor.Restore(before);restoreErr!=nil{writeError(w,500,fmt.Sprintf("Apply failed: %v; restore failed: %v",applyErr,restoreErr));return}
				if restoreErr:=s.ReloadMihomo();restoreErr!=nil{writeError(w,502,fmt.Sprintf("Apply failed: %v; old file restored, recovery apply failed: %v",applyErr,restoreErr));return}
				writeError(w,502,"Apply failed; previous configuration restored: "+applyErr.Error());return
			}
			for _,p:=range s.pendingDeletes{if err:=os.Remove(p);err!=nil&&!os.IsNotExist(err){log.Printf("Cache removal: %v",err)}}
		}
		for k,vals:=range rec.Header(){for _,v:=range vals{w.Header().Add(k,v)}}
		w.WriteHeader(rec.Code);_,_=w.Write(rec.Body.Bytes())
	}
}

func(s *Server) restartNikki()error{
	ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
	if b,err:=exec.CommandContext(ctx,"uci","-q","get","nikki.mixin.mixin_file_content").Output();err!=nil||strings.TrimSpace(string(b))!="1"{return fmt.Errorf("Nikki mixin_file_content must be enabled")}
	if b,err:=exec.CommandContext(ctx,"/etc/init.d/nikki","restart").CombinedOutput();err!=nil{return fmt.Errorf("Nikki restart: %w: %s",err,b)}
	for {
		if s.CheckMihomo(ctx).Alive {
			if eff,err:=config.NewYAMLEditor(s.effectivePath);err==nil{
				ok:=true
				want:=s.editor.GetRules();got:=eff.GetRules()
				if len(got)<len(want)||!slices.Equal(got[:len(want)],want){ok=false}
				for _,g:=range s.editor.GetProxyGroupEntries(){if !slices.Contains(eff.GetProxyGroups(),g.Name){ok=false}}
				if ok{return nil}
			}
		}
		select{case <-ctx.Done():return fmt.Errorf("Nikki did not come up with expected rules");case <-time.After(time.Second):}
	}
}
