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

// Decide and log the result of one resolver's record entries.
// Trigger wins over expected; expected wins over no match at all.
func evaluateRecords(records []string, conf *config) Triool {
	result := Uncertain
	for _, elem := range records {
		log.Println(elem)
		if strings.Contains(elem, conf.ExpectedValue) {
			log.Println("Normal value matched")
			result = False
		}
		if strings.Contains(elem, conf.TriggerValue) {
			// Something happened
			log.Println("Trigger value matched")
			return True
		}
	}
	return result
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
