package dcache

import (
	"os"
	"runtime/pprof"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func init() {
	plugin.Register(name, setup)

}

func setup(c *caddy.Controller) error {
	var log = clog.NewWithPlugin(name)
	for i := 2; i != 0; i-- {
		if !c.Next() {
			return c.SyntaxErr("dcache redishost:port")
		}
	}

	host := c.Val()

	log.Infof("dcache connect to host name %s", host)

	dcache := New(host)
	dcache.log = log

	if err := dcache.connect(); err != nil {
		return plugin.Error(name, err)
	}

	log.Infof("redis connect success")

	go dcache.run()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dcache.Next = next
		return dcache
	})

	f, err := os.Create("/tmp/cpuprofile")
	if err != nil {
		panic(err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Error(err)
	}
	return nil
}
