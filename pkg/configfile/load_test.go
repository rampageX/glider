package configfile

import (
	"github.com/nadoo/conflag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIncludeSemantics(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0700)
	os.WriteFile(filepath.Join(dir, "main.conf"), []byte("value=main-before\nlist=one\ninclude=sub/common.conf\nvalue=main-after\nlist=three\n"), 0600)
	os.WriteFile(filepath.Join(dir, "sub/common.conf"), []byte("value=common\nlist=two\ninclude=child.conf\n"), 0600)
	os.WriteFile(filepath.Join(dir, "sub/child.conf"), []byte("list=two\n"), 0600)
	f := conflag.New("test")
	var value string
	var list []string
	f.StringVar(&value, "value", "default", "")
	f.StringSliceVar(&list, "list", nil, "")
	if e := Parse(f, []string{"-config", filepath.Join(dir, "main.conf"), "-value=cli", "-list=four"}, "test"); e != nil {
		t.Fatal(e)
	}
	if value != "cli" || !reflect.DeepEqual(list, []string{"one", "two", "two", "three", "four"}) {
		t.Fatalf("%s %v", value, list)
	}
	if f.ConfDir() != dir {
		t.Fatal(f.ConfDir())
	}
}
func TestMissingCycleAndManagedInclude(t *testing.T) {
	for _, body := range []string{"include=missing.conf", "include=main.conf", "include=nodes.conf\nsubscription-output=nodes.conf"} {
		dir := t.TempDir()
		file := filepath.Join(dir, "main.conf")
		os.WriteFile(file, []byte(body), 0600)
		os.WriteFile(filepath.Join(dir, "nodes.conf"), []byte("# empty"), 0600)
		f := conflag.New("test")
		f.String("subscription-output", "", "")
		if e := Parse(f, []string{"-config=" + file}, "test"); e == nil {
			t.Fatal("accepted", body)
		}
	}
}
