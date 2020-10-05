package main

type dbInfo struct {
	DBType     string `json:"dbType"`
	DBAddr     string `json:"dbAddr"`
	DBUserName string `json:"dbUsername"`
	DBPassword string `json:"dbPassword"`
}
