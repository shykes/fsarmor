package main

import (
	"github.com/shykes/fsarmor"
	"log"
	"os"
)

func DieUsage() {
	log.Fatalf(`
Usage: %s split|join DIR

  %s split DIR: read a tar archive from STDIN and split it into an armored filesystem tree at DIR.
                  DIR is created if needed.
		  DIR is armored, which means it can be safely copied to any filesystem (including git) without information loss.

  %s join DIR: write a tar archive to STDOUT from the contents of DIR, interpreted as an armored filesystem tree.
`, os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	if len(os.Args) < 2 {
		DieUsage()
	}
	switch os.Args[1] {
	case "split":
		{
			CmdSplit(os.Args[2:]...)
		}
	case "join":
		{
			CmdJoin(os.Args[2:]...)
		}
	default:
		{
			DieUsage()
		}
	}
}

func CmdJoin(args ...string) {
	err := fsarmor.Join(args[0], os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}

func CmdSplit(args ...string) {
	err := fsarmor.Split(os.Stdin, args[0])
	if err != nil {
		log.Fatal(err)
	}
}
