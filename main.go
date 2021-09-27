package main

import (
	"log"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-ping/ping"
	"github.com/levigross/grequests"
)

type TestNode struct {
	Region string
	IP     string
	Stats  *ping.Statistics
}

func main() {
	req, err := grequests.Get("http://ec2-reachability.amazonaws.com/",
		&grequests.RequestOptions{RequestTimeout: time.Second * 10})
	if err != nil {
		log.Fatal(err)
	}
	gq, err := goquery.NewDocumentFromResponse(req.RawResponse)
	if err != nil {
		log.Fatal(err)
	}
	req.Close()
	tr := gq.Find("tr")
	tr.Each(func(i int, s *goquery.Selection) {
		td := s.Find("td")
		if td.Length() == 4 {
			n := TestNode{td.Eq(0).Text(), td.Eq(2).Text(), nil}
			pinger, err := ping.NewPinger(n.IP)
			if err != nil {
				log.Println("New Err:", n.Region, n.IP, err)
				return
			}
			pinger.Debug = true
			pinger.SetPrivileged(true)
			pinger.Interval = time.Millisecond * 10
			pinger.Count = 3
			pinger.Timeout = time.Second * 10

			err = pinger.Run()
			if err != nil {
				log.Println("Run Err:", n.Region, n.IP, err)
				return
			}
			n.Stats = pinger.Statistics()
			log.Println(n.Region, n.IP, n.Stats.AvgRtt)
		}

	})

}
