// Package whoami implements a plugin that returns details about the resolving
// querying it.
package whoami

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/miekg/dns"
)

// Whoami is a plugin that returns your IP address, port and the protocol used for connecting
// to CoreDNS.
type Whoami struct{}

func lookup(dnsQuery string, kubeconfig string) string {
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	namespaces, err := clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	for _, namespace := range namespaces.Items {
		ingresses, err := clientset.ExtensionsV1beta1().Ingresses(namespace.Name).List(metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		for _, ingress := range ingresses.Items {
			matchingIngress := false
			for _, rule := range ingress.Spec.Rules {
				if rule.Host == dnsQuery {
					matchingIngress = true
					break
				}
			}
			ips := ingress.Status.LoadBalancer.Ingress
			if matchingIngress && len(ips) > 0 {
				return ips[0].IP
			}
		}
	}
	return ""
}

// ServeDNS implements the plugin.Handler interface.
func (wh Whoami) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	ip := state.IP()
	var rr dns.RR
	answers := []dns.RR{}

	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	switch state.Family() {
	case 1:
		lookupName := state.QName()
		lookupName = lookupName[0 : len(lookupName)-1]
		ingressIP := lookup(lookupName, kubeconfig)
		if ingressIP == "" {
			log.Errorf("Failed to look up %s", lookupName)
			return dns.RcodeServerFailure, nil
		}
		log.Errorf(" %v -> %v (%v)", lookupName, ingressIP, ip)
		rr = new(dns.A)
		rr.(*dns.A).Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA,
			Class: dns.ClassINET, Ttl: 3600}
		rr.(*dns.A).A = net.ParseIP(ingressIP).To4()
		answers = append(answers, rr)
	case 2:
		log.Errorf("Not sure what case 2 is %s", ip)
		rr = new(dns.AAAA)
		rr.(*dns.AAAA).Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: state.QClass()}
		rr.(*dns.AAAA).AAAA = net.ParseIP(ip)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	m.Answer = answers

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (wh Whoami) Name() string { return "whoami" }
