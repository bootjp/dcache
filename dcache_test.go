package dcache

// Refer to the coredns cache plugin test
// https://github.com/coredns/coredns/blob/002b748ccd6b7cc2e3a65f1bd71509f80b95d342/plugin/cache/cache_test.go

import (
	"context"
	"testing"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type cacheTestCase struct {
	test.Case
	in                 test.Case
	AuthenticatedData  bool
	RecursionAvailable bool
	Truncated          bool
	shouldCache        bool
}

var cacheTestCases = []cacheTestCase{
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("bootjp.me.	3600	IN	A	104.21.15.181"),
				test.A("bootjp.me.	3600	IN	A	195.201.182.103"),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("bootjp.me.	3600	IN	A	104.21.15.181"),
				test.A("bootjp.me.	3600	IN	A	195.201.182.103"),
			},
		},
		shouldCache: true,
	},
	{
		Truncated: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Answer: []dns.RR{test.MX("bootjp.me.	1800	IN	MX	1 aspmx.l.google.com.")},
		},
		in:          test.Case{},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		in: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("bootjp.me.	3600	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "bootjp.me.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("bootjp.me.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("bootjp.me.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("bootjp.me.	3600	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 bootjp.me. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: true,
	},
}

func cacheMsg(m *dns.Msg, tc cacheTestCase) *dns.Msg {
	m.RecursionAvailable = tc.RecursionAvailable
	m.AuthenticatedData = tc.AuthenticatedData
	m.Authoritative = true
	m.Rcode = tc.Rcode
	m.Truncated = tc.Truncated
	m.Answer = tc.in.Answer
	m.Ns = tc.in.Ns
	// m.Extra = tc.in.Extra don't copy Extra, because we don't care and fake EDNS0 DO with tc.Do.
	return m
}

func newTestCache() (*Dcache, dns.ResponseWriter) {
	c := New("127.0.0.1:6379")
	log := clog.P{}

	return c, &ResponseWriter{
		ResponseWriter: nil,
		log:            log,
		cache:          c,
	}
}

func TestCache(t *testing.T) {
	c, crr := newTestCache()
	if err := c.connect(); err != nil {
		t.Fatalf("failed connect %s", err)
	}
	go c.runSubscribe()
	go c.runPublish()

	time.Sleep(time.Second)

	for _, tc := range cacheTestCases {
		m := tc.in.Msg()
		m = cacheMsg(m, tc)

		state := &request.Request{W: &test.ResponseWriter{}, Req: m}
		crr.(*ResponseWriter).state = *state
		ans := &AnswerCache{
			Name:      state.Name(),
			Response:  m,
			Type:      dns.Type(state.QType()),
			Do:        state.Do(),
			TimeToDie: time.Now().UTC().Add(1 * time.Minute).Unix(),
			// By is not set use self cache
		}
		c.queue.Enqueue(ans)

		time.Sleep(2 * time.Second)
		res, eok := c.errorCache.Get(time.Now().UTC().Unix(), state)
		res, sok := c.successCache.Get(time.Now().UTC().Unix(), state)

		if eok != tc.shouldCache && sok != tc.shouldCache {
			t.Errorf("Cached message that should not have been cached: %s type: %s cache err %t success %t expect %t\"", state.Name(), state.Type(), eok, sok, tc.shouldCache)
			continue
		}

		if sok {
			resp := res.Response

			if err := test.Header(tc.Case, resp); err != nil {
				t.Logf("Cache %v", resp)
				t.Error(err)
				continue
			}

			if err := test.Section(tc.Case, test.Answer, resp.Answer); err != nil {
				t.Logf("Cache %v -- %v", test.Answer, resp.Answer)
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Ns, resp.Ns); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Extra, resp.Extra); err != nil {
				t.Error(err)
			}
		}
	}
}

func BenchmarkCacheResponse(b *testing.B) {
	c := New("127.0.0.1:6379")
	c.log = clog.NewWithPlugin("test")
	ctx := context.TODO()

	if err := c.connect(); err != nil {
		panic(err)
	}

	go c.runSubscribe()
	go c.runPublish()
	time.Sleep(5 * time.Second)

	reqs := make([]*dns.Msg, 5)
	for i, q := range []string{"example1", "example2", "a", "b", "ddd"} {
		qname := q + ".example.org."
		reqs[i] = new(dns.Msg)
		reqs[i].SetQuestion(qname, dns.TypeA)
		reqs[i].Question[0].Name = qname
		reqs[i].Question[0].Qtype = dns.TypeA

		resp := &dns.Msg{}
		resp.Answer = []dns.RR{
			test.A(qname + "	3600	IN	A	104.21.15.181"),
			test.A(qname + "	3600	IN	A	195.201.182.103"),
		}

		now := time.Now()

		c.publish(&AnswerCache{
			Name:      qname,
			Response:  resp,
			Type:      dns.Type(dns.TypeA),
			Do:        false,
			TimeToDie: now.Unix() + int64(c.minTTL(resp)),
			By:        "a",
			Error:     false,
		})
	}

	time.Sleep(10 * time.Second)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		for _, req := range reqs {
			c.ServeDNS(ctx, &test.ResponseWriter{}, req)
		}
	}
}
