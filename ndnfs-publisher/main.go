package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	//"io/ioutil"
	"bufio"
	"strings"

	"github.com/go-ndn/log"
	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
	"github.com/go-ndn/persist"
)

type face struct {
	ndn.Face
	log.Logger
}

var (
	configPath = flag.String("config", "ndnfs.json", "config path")
	debug      = flag.Bool("debug", false, "enable logging")
	//filePrefix = flag.String("prefix","/ndn/file","name prefix for shared directory")
	//fileDir	= flag.String("dir","/etc","file directory to share")
)

var (
	key ndn.Key
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

	// create a new face
	recv := make(chan *ndn.Interest)
	face, err := newFace(config.NFD.Network, config.NFD.Address, recv)
	if err != nil {
		log.Fatalln(err)
	}
	defer face.Close()

	// key
	pem, err := os.Open(config.PrivateKeyPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer pem.Close()
	key, err = ndn.DecodePrivateKey(pem)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("key", key.Locator())

	packet_size := 8000

    if config.PacketSize != 0 {
    	packet_size = config.PacketSize
	}

	persist_db := "ndnfs.db"

	if len(config.ContentDB) != 0 {
    	persist_db = config.ContentDB
	}

	// create an interest mux
	m := mux.New()
	// 7. logging
	m.Use(mux.Logger)
	// 6. versioning
	m.Use(mux.Versioner)
	// 5. before encrypting it, zip it
	m.Use(mux.Gzipper)
	// 4. before segmenting it, encrypt it
	m.Use(mux.Encryptor(key.(*ndn.RSAKey)))
	// 3. if the data packet is too large, segment it
	m.Use(mux.Segmentor(packet_size))
	// 2. reply the interest with the on-disk cache
	m.Use(persist.Cacher(persist_db))
	// 1. reply the interest with the in-memory cache
	m.Use(mux.Cacher)
	// 0. an interest packet comes
	m.Use(mux.Queuer)

	fmt.Println("")
	fmt.Println("Prefix = ", config.File.Prefix)
	fmt.Println("")

	files, err := filepath.Glob(config.File.Dir + "/*")
	if err != nil {
		log.Fatalln(err)
	} else {
		fmt.Println()
		fmt.Println("List of files")
		fmt.Println("-------------")
		for i := 0; i < len(files); i++ {

			if IsFile(files[i]) {
				_, filename := filepath.Split(files[i])
				fmt.Println("[", i, "] -", filename)
			}

		}

	}

	m.Handle(FileServer(config.File.Prefix, config.File.Dir))

	// pump the face's incoming interests into the mux
	m.Run(face, recv, key)
}

func newFace(network, address string, recv chan<- *ndn.Interest) (f *face, err error) {
	conn, err := packet.Dial(network, address)
	if err != nil {
		return
	}
	f = &face{
		Face: ndn.NewFace(conn, recv),
	}
	if *debug {
		f.Logger = log.New(log.Stderr, fmt.Sprintf("[%s] ", conn.RemoteAddr()))
	} else {
		f.Logger = log.Discard
	}
	f.Println("face created")
	return
}

func IsFile(f string) (filestatus bool) {

	info, _ := os.Stat(f)

	switch mode := info.Mode(); {
	case mode.IsDir():
		filestatus = false
	case mode.IsRegular():
		filestatus = true
	}

	return
}

func FileServer(from, to string) (string, mux.Handler) {
	return from, mux.HandlerFunc(func(w ndn.Sender, i *ndn.Interest) {
		//content, err := ioutil.ReadFile(to + filepath.Clean(strings.TrimPrefix(i.Name.String(), from)))
		//if err != nil {
		//	return
		//}
		file, err := os.Open(to + filepath.Clean(strings.TrimPrefix(i.Name.String(), from)))
        if err != nil {
                 return
        }

        fileInfo, _ := file.Stat()
        var fileSize int64 = fileInfo.Size()
        bytes := make([]byte, fileSize)

        buffer := bufio.NewReader(file)
        _, err = buffer.Read(bytes)

		w.SendData(&ndn.Data{
			Name:    i.Name,
			Content: bytes,
		})
	})
}
