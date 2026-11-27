package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var blockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("64:ff9b::/96"),
}
func publicIP(ip netip.Addr)bool{
	ip=ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast(){return false}
	for _,p:=range blockedNetworks{if p.Contains(ip){return false}}
	return true
}
func publicPreviewClient()*http.Client{
	dialer:=&net.Dialer{Timeout:8*time.Second}
	tr:=&http.Transport{DisableKeepAlives:true,TLSHandshakeTimeout:8*time.Second,DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
		host,port,err:=net.SplitHostPort(address);if err!=nil{return nil,err}
		ips,err:=net.DefaultResolver.LookupNetIP(ctx,"ip",host);if err!=nil{return nil,err}
		for _,ip:=range ips{if !publicIP(ip){return nil,fmt.Errorf("preview destination must be public")}}
		var last error
		for _,ip:=range ips{c,e:=dialer.DialContext(ctx,network,net.JoinHostPort(ip.String(),port));if e==nil{return c,nil};last=e}
		if last==nil{last=fmt.Errorf("no public preview address")}
		return nil,last
	}}
	return &http.Client{Transport:tr,Timeout:20*time.Second,CheckRedirect:func(req *http.Request,via []*http.Request)error{
		if len(via)>=5|| (req.URL.Scheme!="https"&&req.URL.Scheme!="http")||req.URL.User!=nil{return fmt.Errorf("invalid preview redirect")};return nil
	}}
}
