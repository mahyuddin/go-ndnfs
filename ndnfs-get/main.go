package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-ndn/log"
	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

var (
	configPath = flag.String("config", "ndnfs.json", "config path")
	fileName = flag.String("file", "hosts", "Filename")
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	flag.Parse()

	var data []byte
	var retry int = 0

	// config
	configFile, err := os.Open(*configPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer configFile.Close()

	err = json.NewDecoder(configFile).Decode(&config)
	if err != nil {
		log.Fatalln(err)
	}

	// connect to nfd
	conn, err := packet.Dial(config.NFD.Network, config.NFD.Address)
	if err != nil {
		log.Fatalln(err)
	}
	// start a new face but do not receive new interests
	face := ndn.NewFace(conn, nil)
	defer face.Close()

	// read producer key
	pem, err := os.Open(config.PrivateKeyPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer pem.Close()
	key, _ := ndn.DecodePrivateKey(pem)

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
	dec := mux.Decryptor(key.(*ndn.RSAKey))
	// 6. unzip

    filePrefix := config.File.Prefix + "/" + *fileName
    file, err := os.Create(*fileName)
    if err != nil {
        log.Fatalln(err)
    }
    defer file.Close()

    fmt.Printf("\nFetching file %s from ndn:%s\n\n", *fileName, filePrefix)

    retry_limit := 10

    if config.RetryLimit != 0 {
    	retry_limit = config.RetryLimit
	}

	for retry = 0; retry <= retry_limit; retry++ {
		data = f.Fetch(face, &ndn.Interest{Name: ndn.NewName(filePrefix)}, mux.Assembler, dec, mux.Gunzipper)

		if data != nil {
			break
		}else {
			fmt.Println("Empty data!")
		}
	}

	if retry <= retry_limit {
		databytes, err := io.Copy(file, bytes.NewReader(data))
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Printf("wrote %d bytes\n", databytes)
	} else {
		fmt.Printf("\nFailed to fetch %s file after %d times retry attempt.\n\n", *fileName, retry)
		err := os.Remove(*fileName)
		if err != nil {
        	log.Fatalln(err)
    	}
	}

}
