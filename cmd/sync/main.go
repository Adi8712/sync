package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/internal/indexer"
	"sync/internal/logger"
	"sync/internal/network"
	"syscall"
)

func main() {
	fld := flag.String("folder", "", "path")
	prt := flag.String("port", "9000", "port")
	flg := flag.Parse

	flg()
	if *fld == "" {
		logger.Err("Need -folder")
		os.Exit(1)
	}

	host, _ := os.Hostname()
	id := fmt.Sprintf("%s-%s", host, *prt)
	st := network.NewNetworkState()

	if idx, err := indexer.ScanFolder(*fld); err == nil {
		st.Update(id, idx)
	}

	logger.Info("Starting %s on :%s", id, *prt)
	go network.Start(*fld, ":"+*prt, id, st)
	go network.Broadcast(*prt)
	go network.Discover(id, func(a string) { network.Connect(*fld, a, id, st) })

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(logger.G + logger.C)
		if !scanner.Scan() {
			break
		}

		p := strings.Fields(scanner.Text())
		if len(p) == 0 {
			continue
		}

		switch p[0] {
		case "status":
			status(*fld, id, st)
		case "sync":
			if len(p) < 2 {
				logger.Warn("sync [all|idx]")
				continue
			}
			files := st.GetGlobal()
			if p[1] == "all" {
				local, _ := indexer.ScanFolder(*fld)
				lh := make(map[string]bool)
				for _, f := range local {
					lh[f.Hash] = true
				}
				for _, f := range files {
					if !lh[f.Hash] {
						network.BroadcastReq(f.Hash, f.RelativePath)
					}
				}
			} else if i, _ := strconv.Atoi(p[1]); i >= 0 && i < len(files) {
				network.BroadcastReq(files[i].Hash, files[i].RelativePath)
			}
		case "rename":
			if len(p) < 3 {
				logger.Warn("rename [idx] [name]")
				continue
			}
			i, _ := strconv.Atoi(p[1])
			files := st.GetGlobal()
			if i >= 0 && i < len(files) {
				f := files[i]
				oldP, newP := filepath.Join(*fld, f.RelativePath), filepath.Join(*fld, p[2])
				os.MkdirAll(filepath.Dir(newP), os.ModePerm)
				if err := os.Rename(oldP, newP); err == nil {
					st.SetVote(f.Hash, p[2])
					network.BroadcastVote(f.Hash, p[2])
					logger.Done("Renamed: %s", p[2])
					status(*fld, id, st)
				} else {
					logger.Err("Rename fail: %v", err)
				}
			}
		case "exit":
			os.Exit(0)
		default:
			logger.Warn("status, sync, rename, exit")
		}
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}

func status(fld, id string, st *network.NetworkState) {
	if l, err := indexer.ScanFolder(fld); err == nil {
		st.Update(id, l)
		network.BroadcastIdx(fld, id)
	}

	files := st.GetGlobal()
	local, _ := indexer.ScanFolder(fld)
	lh := make(map[string]bool)
	for _, f := range local {
		lh[f.Hash] = true
	}

	fmt.Println(logger.Y + "\n--- Network ---" + logger.C)
	for i, f := range files {
		stat := logger.G + "OK" + logger.C
		if !lh[f.Hash] {
			stat = logger.R + "MISSING" + logger.C
		}

		col := ""
		if (i > 0 && files[i-1].RelativePath == f.RelativePath) || (i < len(files)-1 && files[i+1].RelativePath == f.RelativePath) {
			col = logger.R + " [COLLISION]" + logger.C
		}

		lbl := logger.G + "Consensus" + logger.C
		if _, ok := st.GetWinner(f.Hash); !ok {
			lbl = logger.Y + "TIE!" + logger.C
		}

		fmt.Printf("[%d] %s (%s) - %s [%s]%s\n", i, f.RelativePath, f.Hash[:8], stat, lbl, col)
	}
	fmt.Println()
}
