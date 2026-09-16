// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

// main 执行代码规范门禁，违规返回非零退出码且保留完整报告。
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 解析独立检查命令，不连接生产服务或修改被检查源码。
func run(arguments []string, output, errors io.Writer) int {
	options := flag.NewFlagSet("quality", flag.ContinueOnError)
	options.SetOutput(errors)
	root := options.String("root", ".", "待检查的项目目录")
	reportPath := options.String("out", "", "JSON 报告路径，留空输出到标准输出")
	if err := options.Parse(arguments); err != nil {
		return 2
	}
	report, err := scanRoot(*root)
	if err != nil {
		fmt.Fprintln(errors, err)
		return 2
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(errors, err)
		return 2
	}
	encoded = append(encoded, '\n')
	if *reportPath != "" {
		err = os.WriteFile(*reportPath, encoded, 0600)
	} else {
		_, err = output.Write(encoded)
	}
	if err != nil {
		fmt.Fprintln(errors, err)
		return 2
	}
	fmt.Fprintf(errors, "Go 文件=%d，函数=%d，违规=%d，状态=%s\n",
		report.Files, report.Functions, len(report.Violations), report.Status)
	if len(report.Violations) > 0 {
		return 1
	}
	return 0
}
