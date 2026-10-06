package main

import (
	"flag"
	"github.com/alexflint/go-filemutex"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	// Parse commandline
	confPath := flag.String("conf", "config.toml", "Configuration file")
	flag.Parse()

	// cd to config directory
	err := os.Chdir(filepath.Dir(*confPath))
	if err != nil {
		log.Panic(err)
	}

	conf, err := loadConfig(*confPath)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("Dead Man's Switch starting...")

	filename := filepath.Join(os.TempDir(), "dmswitch.lock")

	globalMutex, err := filemutex.New(filename)
	if err != nil {
		log.Fatalf("Could not create lock file %s\n", filename)
	}

	log.Println("Trying to acquire global lock...")
	globalMutex.Lock()
	defer globalMutex.Unlock()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	var checkTimer *time.Timer
	var deadline time.Time // wipe countdown; zero = disarmed
	var lastTick time.Time
	for {
		log.Println("Start routine check...")
		now := time.Now()
		ret := check(conf)

		var fire bool
		deadline, fire = advance(deadline, ret, conf, now, lastTick)
		if fire {
			execute(conf)
		}
		lastTick = now

		checkTimer = time.NewTimer(time.Duration(conf.CheckInterval) * time.Second)
		log.Println("Idle...")
		select {
		case <-checkTimer.C:
			continue
		case <-signalChan:
			log.Println("SIGINT received, quitting...")
			os.Exit(0)
		}
	}
}
