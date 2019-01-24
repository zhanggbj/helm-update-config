package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v1"
	"k8s.io/helm/pkg/helm"
	"k8s.io/helm/pkg/strvals"
)

func main() {
	var (
		cliValues   []string
		resetValues bool
	)

	cmd := &cobra.Command{
		Use:   "helm update-config [flags] RELEASE",
		Short: "update config values or templates of an existing release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vals := make(map[string]interface{})
			for _, v := range cliValues {
				if err := strvals.ParseInto(v, vals); err != nil {
					return err
				}
			}

			update := updateConfigCommand{
				client:      helm.NewClient(helm.Host(os.Getenv("TILLER_HOST"))),
				release:     args[0],
				values:      vals,
				resetValues: resetValues,
			}

			return update.run()
		},
	}

	cmd.Flags().StringArrayVar(&cliValues, "set-value", []string{}, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	cmd.Flags().BoolVar(&resetValues, "reset-values", false, "when upgrading, reset the values to the ones built into the chart")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type updateConfigCommand struct {
	client      helm.Interface
	release     string
	values      map[string]interface{}
	resetValues bool
}

func (cmd *updateConfigCommand) run() error {
	// In CFEE, a helm elease name is composed of ${NAMESPACE}.${VERSION_DATE}.${VERSION_TIME}.
	str := strings.Split(cmd.release, ".")
	ns := str[0]
	ls, err := cmd.client.ListReleases(helm.ReleaseListNamespace(ns))
	if err != nil {
		return err
	}

	var base map[interface{}]interface{}
	yaml.Unmarshal([]byte(ls.Releases[0].Config.Raw), &base)
	dest := mergeValues(base, cmd.values)

	var opt helm.UpdateOption
	if cmd.resetValues {
		opt = helm.ResetValues(true)
	} else {
		opt = helm.ReuseValues(true)
	}

	newValues, _ := yaml.Marshal(dest)
	_, err = cmd.client.UpdateReleaseFromChart(
		ls.Releases[0].Name,
		ls.Releases[0].Chart,
		helm.UpdateValueOverrides(newValues),
		opt,
	)

	if err != nil {
		return fmt.Errorf("Error: failed to update release", err)
	}

	fmt.Printf("Info: update successfully\n")
	return nil
}

func mergeValues(dest map[interface{}]interface{}, src map[string]interface{}) map[interface{}]interface{} {
	for k, v := range src {
		// If the key doesn't exist, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}

		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}

		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[interface{}]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If they are bosh map, merge them
		dest[k] = mergeValues(destMap, nextMap)
	}

	return dest
}
