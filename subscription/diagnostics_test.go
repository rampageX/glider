package subscription

import (
 "bytes"
 "context"
 "crypto/x509"
 "errors"
 "io"
 stdlog "log"
 "net"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/nadoo/conflag"
 glog "github.com/nadoo/glider/pkg/log"
)
func TestDiagnosticsSafeAndVerboseOnly(t *testing.T){
 const secret="SECRET_SUBSCRIPTION_TOKEN_AND_PASSWORD"
 cases:=[]struct{body,want string}{
  {"<!DOCTYPE html><html>"+secret+"</html>","stage=envelope"},
  {ss("test")+"\nssr://"+b64("example.test:BAD_"+secret+":origin:aes-128-cfb:plain:cA"),"ssr-port:invalid-number"},
  {ss("test")+"\nss://none:"+secret+"%ZX@example.test:443","ss-userinfo:invalid-percent-escape"},
  {ss("test")+"\nssr://"+b64("example.test:443:origin:aes-128-cfb:plain:cA/?remarks=%ZZ"+secret),"ssr-query"},
 }
 for _,tc:=range cases{
  _,_,err:=Parse([]byte(tc.body))
  if err==nil||!strings.Contains(err.Error(),tc.want)||strings.Contains(err.Error(),secret){t.Fatalf("unsafe or missing diagnostic: %v",err)}
  summary:=bodySummary([]byte(tc.body));if strings.Contains(summary,secret){t.Fatal("body leaked")}
 }
 for _,e:=range []error{
  errors.New(secret),
  &url.Error{Op:"Get",URL:"https://"+secret+"/?token="+secret,Err:context.DeadlineExceeded},
  x509.HostnameError{Host:secret},
  &net.DNSError{Err:secret,Name:secret},
 }{if strings.Contains(networkReason(e),secret){t.Fatal("network error leaked")}}
 label:=routeLabel("socks5h://user:"+secret+"@192.168.2.8:7373#name="+secret)
 if label!="socks5h://192.168.2.8:7373"{t.Fatal(label)}
 oldOutput:=stdlog.Writer();defer stdlog.SetOutput(oldOutput)
 defer glog.Set(false,19)
 var out bytes.Buffer;stdlog.SetOutput(&out)
 glog.Set(false,0);debugf(Config{},"stage=test");if out.Len()!=0{t.Fatal("diagnostics without verbose")}
 glog.Set(true,0);debugf(Config{},"stage=test");if !strings.Contains(out.String(),"[subscription-debug] stage=test"){t.Fatal("no verbose diagnostic")}
}
func TestDebugHTTPResponseDoesNotLeak(t *testing.T){
 const secret="DO_NOT_LOG_RESPONSE_OR_TOKEN"
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/html; private="+secret)
  io.WriteString(w,"<!doctype html><html>"+secret+"</html>")
 }));defer server.Close()
 oldOutput:=stdlog.Writer();defer stdlog.SetOutput(oldOutput);defer glog.Set(false,19)
 var out bytes.Buffer;stdlog.SetOutput(&out);glog.Set(true,0)
 c:=testConfig(t.TempDir(),server.URL+"/?token="+secret);m:=manager(t,c)
 if _,_,_,e:=m.input(context.Background(),c.URLs[0]);e==nil{t.Fatal("accepted HTML")}
 text:=out.String()
 for _,want:=range []string{"status=200","media=text/html","body=html","stage=envelope"}{if !strings.Contains(text,want){t.Fatal("missing "+want)}}
 if strings.Contains(text,secret)||strings.Contains(text,server.URL){t.Fatal("private data leaked")}
}
func TestCacheDefaultOutsideConfigurationDirectory(t *testing.T){
 f:=conflag.New("test");var c Config;RegisterFlags(f,&c)
 f.Set("subscription","https://example.invalid/test")
 f.Set("subscription-output","nodes.conf")
 dir:=t.TempDir();if e:=c.Resolve(dir);e!=nil{t.Fatal(e)}
 if c.CacheDir!=filepath.Join(os.TempDir(),".subscription-cache"){t.Fatal(c.CacheDir)}
 if c.Output!=filepath.Join(dir,"nodes.conf"){t.Fatal(c.Output)}
}
