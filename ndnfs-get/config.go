package main

var config struct {
	NFD struct {
		Network, Address string
	}
	File struct {
		Dir, Prefix string
	}
	PrivateKeyPath string
	RetryLimit int
}
