package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxPreviewBytes = 8 << 20

func validRuleName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\,\x00\r\n")
}

// Resolve only inside the configured rules directory. Existing files must
// resolve inside it even when a symlink is present.
func (s *Server) safeRulePath(path string) (string,error) {
	if path == "" { return "", fmt.Errorf("empty rule path") }
	root, err := filepath.Abs(s.rulesDir)
	if err != nil { return "",err }
	root, err = filepath.EvalSymlinks(root)
	if err != nil { return "",err }
	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
		if path == ".." || strings.HasPrefix(path,".."+string(filepath.Separator)) { return "",fmt.Errorf("rule path escapes rules directory") }
		// Existing configs use rules/<name> relative to Mihomo home.
		prefix := filepath.Base(root)+string(filepath.Separator)
		if strings.HasPrefix(path,prefix) { path = strings.TrimPrefix(path,prefix) }
		path = filepath.Join(root,path)
	}
	path = filepath.Clean(path)
	inside := func(p string) bool { rel,e:=filepath.Rel(root,p); return e==nil && rel!="." && rel!=".." && !strings.HasPrefix(rel,".."+string(filepath.Separator)) }
	if !inside(path) { return "",fmt.Errorf("rule path escapes rules directory") }
	resolved,err:=filepath.EvalSymlinks(path)
	if err==nil {
		if !inside(resolved) { return "",fmt.Errorf("rule symlink escapes rules directory") }
	} else if !os.IsNotExist(err) { return "",err } else {
		// Require an existing, safe parent; do not follow dangling symlinks.
		if _,e:=os.Lstat(path); e==nil { return "",fmt.Errorf("dangling rule symlink") }
		parent,e:=filepath.EvalSymlinks(filepath.Dir(path))
		if e!=nil || (parent!=root && !inside(parent)) { return "",fmt.Errorf("invalid rule parent") }
	}
	return path,nil
}
func readPreviewFile(path string, pathErr error) ([]byte,error) {
	if pathErr != nil { return nil,pathErr }
	info,err:=os.Stat(path)
	if err!=nil { return nil,err }
	if !info.Mode().IsRegular() || info.Size()>maxPreviewBytes { return nil,fmt.Errorf("not a regular previewable file") }
	f,err:=os.Open(path)
	if err!=nil { return nil,err }
	defer f.Close()
	b,err:=io.ReadAll(io.LimitReader(f,maxPreviewBytes+1))
	if len(b)>maxPreviewBytes { return nil,fmt.Errorf("ruleset exceeds preview limit") }
	return b,err
}
func (s *Server) readSafePreview(path string) ([]byte,error) {
	p,e:=s.safeRulePath(path)
	return readPreviewFile(p,e)
}
