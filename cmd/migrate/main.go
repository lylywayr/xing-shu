package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"xing-shu/internal/migration"
)

func main() {
	source := flag.String("source", "", "历史数据目录，只读")
	target := flag.String("target", "", "星枢数据目录")
	flag.Parse()
	if *source == "" || *target == "" {
		flag.Usage()
		os.Exit(2)
	}
	report, err := migration.Run(*source, *target)
	if err != nil {
		log.Fatalf("星枢离线迁移失败: %v", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("生成迁移报告失败: %v", err)
	}
	fmt.Println(string(data))
}
