package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func check(conf *config) Triool {
	var resolvers []*net.Resolver
	if conf.TrySystemResolver {
		resolvers = append(resolvers, getResolver())
	}
	for _, elem := range conf.CustomResolvers {
		resolvers = append(resolvers, getResolverWithServer(elem))
	}

	var checkResult = Uncertain
	ctx := context.Background()
	for _, resolver := range resolvers {
		if checkResult != Uncertain {
			break
		}

		var ret []string
		var err error

		switch strings.ToUpper(conf.RecordType) {
		case "A", "AAAA":
			ret, err = resolver.LookupAddr(ctx, conf.Record)
		case "TXT":
			ret, err = resolver.LookupTXT(ctx, conf.Record)
		default:
			err = errors.New("unsupported record type")
		}

		if err != nil {
			log.Printf("Unable to resolve record: %s\n", err)
		} else {
			log.Println("Result entries:")
			checkResult = evaluateRecords(ret, conf)
		}
	}
	return checkResult
}

// Decide the result of one resolver's record entries.
// Expected value found → False (alive). Record visible but value missing → True
// (starts/resumes the wipe countdown). Lookup errors never reach here.
func evaluateRecords(records []string, conf *config) Triool {
	for _, elem := range records {
		log.Println(elem)
		if strings.Contains(elem, conf.ExpectedValue) {
			log.Println("Normal value matched")
			return False
		}
	}
	log.Println("Expected value not found")
	return True
}

func runScriptIterative(path string) {
	file, err := os.Stat(path)
	if os.IsNotExist(err) {
		log.Printf("File %s not found or insufficient privilege, skipping...\n", path)
	} else {
		if file.IsDir() {
			log.Printf("Entering directory %s\n", path)
			f, _ := os.Open(path)
			files, _ := f.Readdir(-1)
			f.Close()
			filenames := make([]string, len(files))
			for i, v := range files {
				filenames[i] = v.Name()
			}
			sort.Strings(filenames)
			for _, fi := range filenames {
				runScriptIterative(filepath.Join(path, fi))
			}
		} else {
			// is a single file
			log.Printf("Executing %s:\n", path)
			out, err := exec.Command(path).Output()
			log.Print(string(out))
			if err != nil {
				log.Print(err)
			}
		}
	}
}

func delFileIterative(path string) {
	// first we try a remove all method
	err := os.RemoveAll(path)
	// if unable to clean up, we remove as much as we can
	if err == nil {
		log.Printf("%s removed on first try\n", path)
	} else {
		file, err := os.Stat(path)
		if os.IsNotExist(err) {
			log.Printf("%s not found or insufficient privilege, skipping...\n", path)
		} else {
			if file.IsDir() {
				log.Printf("Entering directory %s\n", path)
				f, _ := os.Open(path)
				files, _ := f.Readdir(-1)
				f.Close()
				for _, fi := range files {
					delFileIterative(filepath.Join(path, fi.Name()))
				}
			}
			os.Remove(path)
			if err != nil {
				log.Print(err)
			}
		}
	}
}

// Countdown state machine, the wipe decision. Called once per poll with the
// pre-check snapshot `now` and the previous iteration's `lastTick`.
//   - False: expected value seen, cancel the countdown.
//   - True:  record visible but value missing — arm one full countdown; when it
//     has expired, fire (and re-arm while still missing, so a persistent
//     missing value fires at most once per period).
//   - Uncertain: lookup failed everywhere — never arms; an armed countdown is
//     frozen by exactly the elapsed poll period (an outage never wipes).
//
// deadline == zero time means disarmed.
func advance(deadline time.Time, ret Triool, conf *config, now, lastTick time.Time) (time.Time, bool) {
	countdown := time.Duration(conf.Countdown) * time.Second
	switch ret {
	case True:
		if deadline.IsZero() {
			log.Printf("Expected value missing; wipe countdown armed until %s", now.Add(countdown).Format(time.RFC3339))
			return now.Add(countdown), false
		}
		if !now.Before(deadline) {
			log.Println("Countdown expired, executing...")
			return now.Add(countdown), true
		}
		log.Printf("Countdown running, %s until wipe", deadline.Sub(now))
		return deadline, false
	case False:
		if !deadline.IsZero() {
			log.Println("Expected value seen; countdown reset")
			return time.Time{}, false
		}
		return deadline, false
	default: // Uncertain
		if deadline.IsZero() {
			log.Println("Record lookup failed; no countdown running")
			return deadline, false
		}
		deadline = deadline.Add(now.Sub(lastTick))
		log.Printf("Record lookup failed; countdown frozen until %s", deadline.Format(time.RFC3339))
		return deadline, false
	}
}

func execute(conf *config) {
	log.Println("Executing hooks...")

	// execute hooks
	for _, entry := range conf.ExecuteScripts {
		runScriptIterative(entry)
	}

	// delete files
	for _, entry := range conf.DeleteFiles {
		delFileIterative(entry)
	}

	if conf.ExitAfterTrigger {
		log.Println("Job done, RIP")
		os.Exit(0)
	}
}
