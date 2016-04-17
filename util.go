package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"sync"
)

var tmpl = template.Must(template.New("foo").Parse(js))
var js = `
<!DOCTYPE HTML>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>Gochart - spline | CodeG.cn</title>
<script type="text/javascript" src="http://cdn.hcharts.cn/jquery/jquery-1.8.3.min.js"></script>
<script type="text/javascript">
$(function () {
	$('#container').highcharts({
		chart: {
			zoomType: 'x',
			spacingRight: 20
		},
		title: {
			text: 'decred dollas',
		},
		xAxis: {
			title: {
				text: 'block'
			},
		},
		yAxis: {
			title: {
				text: 'dcr'
			},
			plotLines: [{
				value: 0,
				width: 1,
				color: '#808080'
			}]
		},
		tooltip: {
			shared: true,
			valueSuffix: 'dcr'
		},
		legend: {
			layout: 'vertical',
			align: 'right',
			verticalAlign: 'middle',
			borderWidth: 0
		},
		series: {{.DataArray}}
	});
});
</script>
</head>
<body>
<script type="text/javascript" src="http://code.highcharts.com/highcharts.js"></script>
<div id="container" style="min-width: 310px; height: 400px; margin: 0 auto"></div>
</body>
</html>
`

var (
	txns []txn
)

type blockSorted []txn

func (bs blockSorted) Less(i, j int) bool { return bs[i].Block < bs[j].Block }
func (bs blockSorted) Swap(i, j int)      { bs[i], bs[j] = bs[j], bs[i] }
func (bs blockSorted) Len() int           { return len(bs) }

func main() {
	out := flag.String("o", "txns.log", "output file name")
	in := flag.String("i", "", "input file name")
	flag.Parse()

	if *in != "" {
		f, err := os.Open(*in)
		if err != nil {
			log.Fatal("error opening input:", err)
		}
		err = json.NewDecoder(f).Decode(&txns)
		log.Println("loaded txns from file", *in, len(txns))
		f.Close()
	} else {
		loadtxns(*in, *out)
	}

	maturity()

	http.HandleFunc("/", looky(*in, *out))
	err := http.ListenAndServe(":5000", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func tickets() {
	cmd := "dcrctl"
	hashesjason, err := exec.Command(cmd, "--wallet", "gettickets", "0").Output()
	if err != nil {
		log.Fatal("couldn't run gettickets", err)
	}

	var hashes struct {
		H []string `json:"hashes"`
	}
	err = json.Unmarshal(hashesjason, &hashes)
	if err != nil {
		log.Fatal("error opening jason of tickets", err)
	}

	var tix int
	var fees, total float64
	prices := make(map[float64]int)

	for _, h := range hashes.H {
		hashjason, err := exec.Command(cmd, "--wallet", "gettransaction", h).Output()
		if err != nil {
			log.Fatal("couldn't run gettransaction, err:", err, "tx:", h)
		}

		var hash struct {
			A float64 `json:"amount"`
			F float64 `json:"fee"`
			//T `blocktime`
		}
		err = json.Unmarshal(hashjason, &hash)
		if err != nil {
			log.Fatal("couldn't decode hash jason", err)
		}
		prices[hash.A] += 1
		fees += hash.F
		total += hash.A
		tix++
	}

	sorted := make([]float64, 0, len(prices))
outer:
	for p := range prices {
		for i, s := range sorted {
			if p < s {
				sorted = append(sorted[:i], append([]float64{p}, sorted[i:]...)...)
				continue outer
			}
		}
		sorted = append(sorted, p)
	}

	for _, p := range sorted {
		fmt.Printf("tix@ %.2f: %d\n", p, prices[p])
	}
	fmt.Println()
	reward := 1.8167 // TODO command for this?
	avg := total / float64(tix)
	fmt.Printf("# tix: %d\n", tix)
	fmt.Printf("total dcr: %.2f\n", total)
	fmt.Printf("avg ticket: %.2f\n", avg)
	fmt.Printf("fees: %.2f\n", fees)
	fmt.Printf("avg fee: %.2f\n", fees/float64(tix))
	fmt.Printf("expected raw return: %.2f\n", reward*float64(tix))
	fmt.Printf("expected %% return: %.2f%%\n", (reward/avg)*100)
}

func listtransactions() []string {
	xcmd := exec.Command("bash", "-c", "dcrctl --wallet listtransactions '*' 999999999")
	txsjason, err := xcmd.CombinedOutput()
	if err != nil {
		log.Fatal("couldn't run listtransactions, err:", err, "out:", string(txsjason), "cmd:", xcmd.Args)
	}

	var txs []struct {
		Tx string `json:"txid"`
	}
	err = json.Unmarshal(txsjason, &txs)
	if err != nil {
		log.Fatal("error opening jason of tickets", err)
	}

	strs := make([]string, len(txs))
	for i, tx := range txs {
		strs[i] = tx.Tx
	}
	return strs
}

type txn struct {
	Block         uint64 `json:"blockheight"`
	Confirmations uint64 `json:"confirmations"`
	Vin           []struct {
		Tx    string `json:"txid"`
		Block uint64 `json:"blockheight"`
	} `json:"vin"`
	Vout []struct {
		Value float64 `json:"value"`
		Tx    struct {
			ASM  string `json:"asm"`
			Type string `json:"type"`
		} `json:"scriptPubKey"`
	} `json:"vout"`
}

type simpletxn struct {
	Value float64 `json:"value"`
}

func loadtxns(in, out string) {
	log.Println("didn't find any existing txns, grabbing them..")
	var wg sync.WaitGroup
	threads := 10
	ch := make(chan string, threads)
	wg.Add(threads)

	for i := 0; i < threads; i++ {
		go func() {
			defer wg.Done()
			for tx := range ch {
				var txn txn
				txjason, err := exec.Command("dcrctl", "--wallet", "getrawtransaction", tx, "1").CombinedOutput()
				if err != nil {
					log.Fatal("couldn't get transaction, err:", err, "tx:", tx)
				}
				err = json.Unmarshal(txjason, &txn)
				if err != nil {
					log.Fatal("couldn't decode txn jason, err:", err, "out:", string(txjason))
				}

				txns = append(txns, txn)
			}
		}()
	}

	txs := listtransactions()
	for _, tx := range txs {
		ch <- tx
	}
	close(ch)
	wg.Wait()

	if out != "" {
		f, err := os.Create(out)
		if err != nil {
			log.Fatal("error opening input:", err)
		}
		err = json.NewEncoder(f).Encode(&txns)
		f.Close()
		log.Println("wrote txns to file", out)
	}
	sort.Sort(blockSorted(txns))
}

func tickets2() []map[string]interface{} {
	points := make(map[string][][2]float64)
	totals := make(map[string]float64)

	for _, txn := range txns {
		if len(txn.Vout) > 0 {
			for _, vout := range txn.Vout {
				if vout.Value == 0 || vout.Tx.Type == "sstxchange" {
					continue
				}
				total := totals[vout.Tx.Type] + vout.Value
				points[vout.Tx.Type] = append(points[vout.Tx.Type], [2]float64{float64(txn.Block), total})
				totals[vout.Tx.Type] = total
				// TODO use different types as +/- to keep a running total of spendable/total/locked/etc
				switch vout.Tx.Type {
				default:
					//case "stakesubmission", "stakegen":
					// fmt.Println(txn.Block, txn.Confirmations, vout.Tx.Type, vout.Value, tx)
				}
			}
		}
	}

	var datas []map[string]interface{}
	for key, val := range points {
		datas = append(datas, map[string]interface{}{"name": key, "connectNulls": true, "data": val})
	}

	return datas
}

func looky(in, out string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("doin it")
		datas := tickets2()
		fmt.Println("got it")
		args := map[string]interface{}{"DataArray": datas}

		tmpl.Execute(os.Stdout, args)
		if err := tmpl.Execute(w, args); err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		}
	}
}

func maturity() {
	var votey []int // time to vote, in blocks, as series

	for _, txn := range txns {
		if len(txn.Vout) > 0 {
		outer:
			for _, vout := range txn.Vout {
				if vout.Tx.Type == "stakegen" {
					for _, vin := range txn.Vin {
						if vin.Block > 0 {
							votey = append(votey, int(txn.Block-vin.Block)) // time to vote (in blocks)
							break outer
						}
					}
					// TODO get vin, get tx of ticket purchase value
				}
			}
		}
	}

	if len(votey) < 1 {
		log.Fatal("no votes found, no stats for you")
	}

	var min, max, mean int
	for _, v := range votey {
		if min == 0 || v < min {
			min = v
		}
		if v > max {
			max = v
		}
		mean += v
	}

	mean = mean / len(votey)
	sort.Sort(sort.IntSlice(votey)) // TODO meh
	median := votey[len(votey)/2]

	btod := func(i int) int { return i / 256 }
	//fmt.Println("live", live)
	fmt.Println("voted", len(votey))
	fmt.Println("median", median, btod(median))
	fmt.Println("mean", mean, btod(mean))
	fmt.Println("min", min, btod(min))
	fmt.Println("max", max, btod(max))
	// TODO 95 99 percentiles
}
