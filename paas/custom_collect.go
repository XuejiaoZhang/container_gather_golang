package paas

import (
	//"errors"
	"fmt"
	"github.com/golang/glog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	//	"time"
	//	"github.com/golang/glog"
	//	info "github.com/google/cadvisor/info/v1"
)

//Gte net info
func GetNet(containerName string) map[string]int64 {
	if containerName != "/" {
		lines := strings.Split(string(containerName), "/")
		if len(lines) > 2 {
			netout, err := exec.Command("/bin/sh", "-c", `docker exec -i `+lines[2]+` netstat -s`).Output()
			//fmt.Println("netitem 1 out ", containerName, netout)
			if len(netout) == 0 {
				/*
					if err != nil {
						fmt.Println("failed to execute %q command: %v", "docker exec netstat -s", err)
						return nil
					}
				*/
				//fmt.Println(containerName, "failed to execute command:", "docker exec netstat -s ", err)
				glog.Infof("%s failed to execute %q command: %v", containerName, "docker exec netstat -s", err)
				return nil
			}
			netItem := getNetItem(netout)
			return netItem
		}
	} else {
		netout, err := exec.Command("/bin/sh", "-c", `netstat -s`).Output()
		if err != nil {
			return nil
		}
		netItem := getNetItem(netout)
		return netItem
	}
	netItem := make(map[string]int64)
	return netItem
}

func getNetItem(netout []byte) map[string]int64 {
	netItem := make(map[string]int64)
	re_ip_received := "([0-9]*) total packets received"
	ip_received := regrexNet(netout, re_ip_received)
	netItem["ip_received"] = ip_received
	re_ip_discarded := "([0-9]*) incoming packets discarded"
	ip_discarded := regrexNet(netout, re_ip_discarded)
	netItem["ip_discarded"] = ip_discarded
	re_tcp_received := "([0-9]*) segments received"
	tcp_received := regrexNet(netout, re_tcp_received)
	netItem["tcp_received"] = tcp_received
	re_tcp_sendout := "([0-9]*) segments send out"
	tcp_sendout := regrexNet(netout, re_tcp_sendout)
	netItem["tcp_sendout"] = tcp_sendout
	re_tcp_activeopen := "([0-9]*) active connections openings"
	tcp_activeopen := regrexNet(netout, re_tcp_activeopen)
	netItem["tcp_activeopen"] = tcp_activeopen
	re_tcp_bad_segments := "([0-9]*) bad segments received"
	tcp_bad_segments := regrexNet(netout, re_tcp_bad_segments)
	netItem["tcp_bad_segments"] = tcp_bad_segments
	return netItem
}

func regrexNet(netout []byte, re_str string) int64 {
	re, _ := regexp.Compile(re_str)
	submatch := re.FindSubmatch(netout)
	num, err := strconv.ParseInt(string(submatch[1]), 10, 64)
	//fmt.Println("FindSubmatch", submatch)
	//fmt.Println("FindSubmatch submatch[1]", string(submatch[1]))
	//fmt.Println("FindSubmatch num", num)
	if err != nil {
		glog.Warningf("Unable to parse integer from %s: %v", submatch[1], err)
	}
	/*
		for _, v := range submatch {
			fmt.Println("v", string(v))
		}
	*/
	return num
}

//Gte IP info
func GetIP(containerName string) map[string]string {
	if containerName != "/" {
		lines := strings.Split(string(containerName), "/")
		if len(lines) > 2 {
			ipout, err := exec.Command("/bin/sh", "-c", `docker inspect `+lines[2]+` |grep "\"IPAddress"`).Output()
			//fmt.Println("ipout 1", containerName, ipout)
			if len(ipout) == 0 {
				fmt.Println(containerName, "failed to execute command:", "docker inspect ", err)
				glog.Infof("%s failed to execute %q command: %v", containerName, "docker inspect ", err)
				return nil
			}
			ipItem := getContainerIPItem(ipout)
			return ipItem
		}
	} else {
		ipout, err := exec.Command("/bin/sh", "-c", `ifconfig|grep "inet .* broadcast"`).Output()
		if err != nil {
			return nil
		}
		//fmt.Println("ipout 2", containerName, ipout)
		ipItem := getMachineIPItem(ipout)
		//fmt.Println("ipItem", ipItem)
		return ipItem
	}
	ipItem := make(map[string]string)
	return ipItem
}

func getMachineIPItem(ipout []byte) map[string]string {
	ipItem := make(map[string]string)
	re_ip := "inet (.*) netmask"
	ip := regrexString(ipout, re_ip)
	ipItem["IP"] = ip
	return ipItem
}

func getContainerIPItem(ipout []byte) map[string]string {
	ipItem := make(map[string]string)
	re_ip := "IPAddress\": \"(.*)\","
	ip := regrexString(ipout, re_ip)
	ipItem["IP"] = ip
	return ipItem
}

func regrexString(ipout []byte, re_str string) string {
	re, _ := regexp.Compile(re_str)
	submatch := re.FindSubmatch(ipout)
	return string(submatch[1])
}
