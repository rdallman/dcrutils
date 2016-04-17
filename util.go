package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

func main() {
	cmd := "./dcrctl"
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
