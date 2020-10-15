package main

type dbInfo struct {
	Type     string `json:"type"`
	Addr     string `json:"addr"`
	Database string `json:"database"`
	UserName string `json:"username"`
	Password string `json:"password"`
}
