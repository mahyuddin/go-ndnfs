package main

import (
	"fmt"
	"os"
	"flag"
	"strings"

	"github.com/go-ndn/log"
	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

var (
	filePrefix = flag.String("prefix","/ndn/file/hosts","name prefix for shared file")
)


func check(e error) {
    if e != nil {
        panic(e)
    }
}

func main() {
	flag.Parse()

	// connect to nfd
	conn, err := packet.Dial("unix", "/tmp/nfd.sock")
	if err != nil {
		log.Fatalln(err)
	}
	// start a new face but do not receive new interests
	face := ndn.NewFace(conn, nil)
	defer face.Close()

	// create a data fetcher
	f := mux.NewFetcher()
	// 0. a data packet comes
	// 1. verifiy checksum
	f.Use(mux.ChecksumVerifier)
	// 2. add the data to the in-memory cache
	f.Use(mux.Cacher)
	// 3. logging
	f.Use(mux.Logger)
	// see producer
	// 4. assemble segments if the content has multiple segments
	// 5. decrypt
	dec := mux.AESDecryptor([]byte("example key 1234"))

	// 6. unzip
	// note: middleware can be both global and local to one handler
	data := f.Fetch(face, &ndn.Interest{Name: ndn.NewName(*filePrefix)}, mux.Assembler, dec, mux.Gunzipper)

	fileSplit := strings.Split(*filePrefix,"/")
	fileName := fileSplit[len(fileSplit)-1] 
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()
	
	databytes, err := file.Write(data)
    	check(err)
    	fmt.Printf("wrote %d bytes\n", databytes)
	file.Sync()
}
