package server

import (
	"os"
	"path/filepath"
	"testing"
)
func TestSafeRulePath(t *testing.T) {
	root:=t.TempDir()
	dir:=filepath.Join(root,"rules")
	if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
	outside:=filepath.Join(root,"config.yaml")
	if err:=os.WriteFile(outside,[]byte("secret"),0600);err!=nil {t.Fatal(err)}
	s:=&Server{rulesDir:dir}
	for _,p:=range []string{"../config.yaml",outside} { if _,err:=s.safeRulePath(p);err==nil {t.Fatalf("accepted %s",p)} }
	if err:=os.Symlink(outside,filepath.Join(dir,"bad.mrs"));err!=nil {t.Fatal(err)}
	if _,err:=s.safeRulePath("bad.mrs");err==nil {t.Fatal("accepted escaping symlink")}
	if _,err:=s.safeRulePath("rules/youtube.mrs");err!=nil {t.Fatal(err)}
}
