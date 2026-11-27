package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"gopkg.in/yaml.v3"
)

// Reload the editable source, not Nikki's generated runtime config.
func (e *YAMLEditor) ReloadFromDisk() error {
	e.mu.Lock(); defer e.mu.Unlock()
	b,err:=os.ReadFile(e.path);if err!=nil{return err}
	n,err:=parseMapping(b);if err!=nil{return err}
	e.node=n
	return nil
}
func parseMapping(b []byte)(*yaml.Node,error){
	var n yaml.Node
	if err:=yaml.Unmarshal(b,&n);err!=nil{return nil,err}
	if len(n.Content)==0 {n=yaml.Node{Kind:yaml.DocumentNode,Content:[]*yaml.Node{{Kind:yaml.MappingNode,Tag:"!!map"}}}}
	if n.Content[0].Kind!=yaml.MappingNode{return nil,fmt.Errorf("expected YAML mapping")}
	return &n,nil
}
func AtomicBytes(path string,b []byte)error{
	f,err:=os.CreateTemp(filepath.Dir(path),".miru-*");if err!=nil{return err}
	defer os.Remove(f.Name())
	if _,err=f.Write(b);err!=nil{f.Close();return err}
	if err=f.Sync();err!=nil{f.Close();return err}
	if err=f.Close();err!=nil{return err}
	return os.Rename(f.Name(),path)
}
func(e *YAMLEditor) Restore(b []byte)error{
	n,err:=parseMapping(b);if err!=nil{return err}
	e.mu.Lock();defer e.mu.Unlock()
	if err=AtomicBytes(e.path,b);err!=nil{return err}
	e.node=n;return nil
}
func(e *YAMLEditor) SetRuleNoResolve(name string)error{
	e.mu.Lock();defer e.mu.Unlock()
	s:=e.readSection("rules")
	if s!=nil{for _,n:=range s.Content{
		p:=strings.Split(n.Value,",")
		if len(p)>=3 && p[0]=="RULE-SET" && p[1]==name {
			for _,part:=range p[3:] {if strings.TrimSpace(part)=="no-resolve"{return nil}}
			n.Value+=",no-resolve";return nil
		}
	}}
	return fmt.Errorf("rule %q not found",name)
}
