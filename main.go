package main

import (
	"flag"
	"github.com/broadinstitute/disk-manager/logs"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

type args struct {
	local      bool
	kubeconfig *string
	configFile *string
}

func main() {
	args := parseArgs()

	m, err := newDiskManager(args)
	if err != nil {
		logs.Error.Fatal(err)
	}

	err = m.addPoliciesToDisks()
	if err != nil {
		logs.Error.Fatal(err)
	}
}

/* Parse command-line arguments */
func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	configFile := flag.String("config-file", "/etc/disk-manager/config.yaml", "path to yaml file with disk-manager config")
	flag.Parse()
	return &args{*local, kubeconfig, configFile}
}
