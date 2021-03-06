package main

import (
	//"net/http"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	//"io/ioutil"
	"bufio"
	"strings"
	"sync"
	"time"

	//"github.com/go-ndn/health"
	"github.com/go-ndn/log"
	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
	"github.com/go-ndn/persist"
)

var (
	configPath = flag.String("config", "ndnfs.json", "config path")
	debug      = flag.Bool("debug", false, "enable logging")
	// filePrefix = flag.String("prefix","/ndn/file","name prefix for shared directory")
	// fileDir	= flag.String("dir","/etc","file directory to share")
)

func main() {

	flag.Parse()

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

	// health monitor
	//go http.ListenAndServe("localhost:8081", nil)

	// connect to nfd
	conn, err := packet.Dial(config.NFD.Network, config.NFD.Address)
	if err != nil {
		log.Fatalln(err)
	}

	// create a new face
	recv := make(chan *ndn.Interest)
	face := ndn.NewFace(conn, recv)
	defer face.Close()

	// key
	pem, err := os.Open(config.PrivateKeyPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer pem.Close()
	key, _ := ndn.DecodePrivateKey(pem)

	packetSize := 8192

	if config.PacketSize != 0 {
		packetSize = config.PacketSize
	}

	persistDB := "ndnfs.db"

	if len(config.ContentDB) != 0 {
		persistDB = config.ContentDB
	}

	persistCache, _ := persist.New(persistDB)

	// Pre generate data packets
	publisher := mux.NewPublisher(persistCache)
	//versioning first
	publisher.Use(mux.Versioner)
	// compress
	publisher.Use(mux.Gzipper)
	// after compress, segment
	publisher.Use(mux.Segmentor(packetSize))

	publishFiles(publisher)

	// create an interest mux
	m := mux.New()
	// 7. logging
	m.Use(mux.Logger)
	// 6. versioning
	m.Use(mux.Versioner)
	// 5. before encrypting it, zip it
	m.Use(mux.Gzipper)
	// 4. before segmenting it, encrypt it
	m.Use(mux.Encryptor("/producer/encrypt", key.(*ndn.RSAKey)))
	// 3. if the data packet is too large, segment it
	m.Use(mux.Segmentor(packetSize))
	// 2. reply the interest with the on-disk cache
	//m.Use(persist.Cacher(persistDB))
	m.Use(mux.RawCacher(persistCache, false))

	// 1. reply the interest with the in-memory cache
	m.Use(mux.Cacher)
	// 0. an interest packet comes
	m.Use(mux.Queuer)

	//m.Use(health.Logger("health", "health.db"))

	// serve encryption key from cache
	m.HandleFunc("/producer/encrypt", func(w ndn.Sender, i *ndn.Interest) {})

	m.Handle(fileServer(config.File.Prefix, config.File.Dir))

	// pump the face's incoming interests into the mux
	m.Run(face, recv, key)
}

func isFile(f string) (filestatus bool) {

	info, _ := os.Stat(f)

	switch mode := info.Mode(); {
	case mode.IsDir():
		filestatus = false
	case mode.IsRegular():
		filestatus = true
	}

	return
}

func fileServer(from, to string) (string, mux.Handler) {
	return from, mux.HandlerFunc(func(w ndn.Sender, i *ndn.Interest) {

		file, err := os.Open(to + filepath.Clean(strings.TrimPrefix(i.Name.String(), from)))

		if err != nil {
			return
		}

		fileInfo, _ := file.Stat()
		fileSize := fileInfo.Size()
		bytes := make([]byte, fileSize)

		buffer := bufio.NewReader(file)
		_, err = buffer.Read(bytes)

		w.SendData(&ndn.Data{
			Name:    i.Name,
			Content: bytes,
		})
	})
}

func insertData(prefix, fileName string) *ndn.Data {

	fmt.Printf("- Publishing %v to %v\n", fileName, prefix)

	file, _ := os.Open(fileName)
	fileInfo, _ := file.Stat()
	fileSize := fileInfo.Size()
	bytes := make([]byte, fileSize)

	buffer := bufio.NewReader(file)
	buffer.Read(bytes)

	return &ndn.Data{
		Name:    ndn.NewName(prefix),
		Content: bytes,
	}
}

func publishFiles(publisher *mux.Publisher) {
	var wg sync.WaitGroup
	
	fmt.Println()
	fmt.Println("go-ndnfs Publisher")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("Prefix = ", config.File.Prefix)
	fmt.Println("Directory = ", config.File.Dir)

	files, err := filepath.Glob(config.File.Dir + "/*")
	if err != nil {
		log.Fatalln(err)
	} else {
		fmt.Println()
		fmt.Println("List of files")
		fmt.Println("-------------")
		
		for i := 0; i < len(files); i++ {

			if isFile(files[i]) {
				wg.Add(1)
				go func (file string)  {
					defer wg.Done()
					_, fileName := filepath.Split(file)
					publisher.Publish(insertData(config.File.Prefix+"/"+fileName, file))
				}(files[i])
			}
		}
		time.Sleep(500 * time.Millisecond)
		fmt.Printf("Waiting...")
		wg.Wait()
		fmt.Printf("\rPre generating data packets process is done.\n")
	}
}
