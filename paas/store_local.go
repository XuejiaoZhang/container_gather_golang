package paas

import (
	"encoding/json"
	"fmt"
	"os"
)

func StoreIntoLocalFile(gatherstats []GatherStats, localfile string) (string, error) {
	if len(gatherstats) < 1 {
		msg := "none gatherstats to store"
		return msg, nil
	}
	file, err := os.OpenFile(localfile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		return "Failed:open local file", err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	err_enc := enc.Encode(gatherstats)
	file.Sync()
	if err_enc != nil {
		return "Failed:store data into local file", err_enc
	}
	msg := "store data into local file seccessfully"
	return msg, nil
}

func ParseFromLocalFile(localfile string) ([]GatherStats, error) {
	var gatherstats_all []GatherStats
	if Exist(localfile) {
		fp, err := os.Open(localfile)
		if err != nil {
			fmt.Println("Failed:open local file", err)
			return gatherstats_all, err
		}
		dec := json.NewDecoder(fp)
		for {
			var gatherstats []GatherStats
			err := dec.Decode(&gatherstats)
			if err != nil {
				break
			}
			gatherstats_all = append(gatherstats_all, gatherstats...)
		}
	}
	return gatherstats_all, nil
}

func Exist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
