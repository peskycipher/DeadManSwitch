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
	"strconv"
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
		case "TXT", "A", "AAAA":
			// Operator directive: A/AAAA resolve as TXT — LookupAddr (reverse PTR)
			// was wrong for these and is gone.
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
// pre-check snapshot `now`.
//   - False: expected value seen — cancel, no matter what (even past expiry).
//   - True:  record visible but value missing — arm one full countdown; fire
//     when expired, and re-arm while still missing (fires at most once per
//     period).
//   - Uncertain: lookup failed everywhere — never arms; an armed deadline keeps
//     running on the wall clock, and expiry still fires (a lookup outage
//     cannot defer the wipe).
//
// deadline == zero time means disarmed.
func advance(deadline time.Time, ret Triool, conf *config, now time.Time) (time.Time, bool) {
	countdown := time.Duration(conf.Countdown) * time.Second
	if ret == False {
		if !deadline.IsZero() {
			log.Println("Expected value seen; countdown reset")
			return time.Time{}, false
		}
		return deadline, false
	}
	if !deadline.IsZero() && !now.Before(deadline) {
		log.Println("Countdown expired, executing...")
		return now.Add(countdown), true
	}
	if ret == True {
		if deadline.IsZero() {
			log.Printf("Expected value missing; wipe countdown armed until %s", now.Add(countdown).Format(time.RFC3339))
			return now.Add(countdown), false
		}
		log.Printf("Countdown running, %s until wipe", deadline.Sub(now))
		return deadline, false
	}
	// Uncertain, disarmed: nothing to do.
	log.Println("Record lookup failed; no countdown running")
	return deadline, false
}

// Persisted deadline so a crash/restart/reboot does not disarm the wipe.
// Unix seconds in a small file next to the lock; guarded by the same global
// lock the running process holds.
var deadlineFile = filepath.Join(os.TempDir(), "dmswitch.countdown")

func loadDeadline() time.Time {
	data, err := os.ReadFile(deadlineFile)
	if err != nil {
		return time.Time{} // no file = disarmed
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{} // garbage = disarmed
	}
	return time.Unix(secs, 0)
}

func saveDeadline(t time.Time) {
	var err error
	if t.IsZero() {
		err = os.Remove(deadlineFile)
	} else {
		err = os.WriteFile(deadlineFile, []byte(strconv.FormatInt(t.Unix(), 10)), 0600)
	}
	if err != nil {
		log.Printf("Could not persist countdown: %s", err)
	}
}

func execute(conf *config) {
	if conf.DryRun {
		log.Println("Dry run: switch fired; destructive actions simulated")
		for _, entry := range conf.ExecuteScripts {
			log.Printf("dry run: would execute: %s", entry)
		}
		for _, entry := range conf.DeleteFiles {
			log.Printf("dry run: would delete: %s", entry)
		}
		if conf.ExitAfterTrigger {
			log.Println("Dry run complete, would exit (exit_after_trigger=true)")
			return
		}
		return
	}

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
