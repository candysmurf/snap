/*
# testing
go test -v github.com/intelsdilabs/pulse/plugin/collector/pulse-collector-facter/facter
*/
package facter

import (
	"strings"
	"testing"

	"github.com/intelsdilabs/pulse/control/plugin"
	. "github.com/smartystreets/goconvey/convey"
)

// fact expected to be available on every system
// can be allways received from Facter for test purposes
const existingFact = "kernel"

var existingNamespace = []string{vendor, prefix, existingFact}

func TestFacterCollectMetrics(t *testing.T) {
	Convey("TestFacterCollect tests", t, func() {

		Convey("asked for nothgin returns nothing", func() {
			f := NewFacter()
			metricTypes := []plugin.PluginMetricType{}
			metrics, err := f.CollectMetrics(metricTypes)
			So(err, ShouldBeNil)
			So(metrics, ShouldBeEmpty)
		})

		Convey("asked for somehting returns somthing", func() {
			f := NewFacter()
			metricTypes := []plugin.PluginMetricType{
				*plugin.NewPluginMetricType(
					existingNamespace,
				),
			}
			metrics, err := f.CollectMetrics(metricTypes)
			So(err, ShouldBeNil)
			So(metrics, ShouldNotBeEmpty)
		})
	})
}

func TestFacterGetMetricsTypes(t *testing.T) {

	Convey("GetMetricTypes functionallity", t, func() {

		f := NewFacter()

		Convey("GetMetricsTypes returns no error", func() {
			// exectues without error
			metricTypes, err := f.GetMetricTypes()
			So(err, ShouldBeNil)
			Convey("metricTypesReply should contain more than zero metrics", func() {
				So(metricTypes, ShouldNotBeEmpty)
			})

			Convey("at least one metric contains metric namespace \"intel/facter/kernel\"", func() {

				expectedNamespaceStr := strings.Join(existingNamespace, "/")

				found := false
				for _, metricType := range metricTypes {
					// join because we cannot compare slices
					if strings.Join(metricType.Namespace(), "/") == expectedNamespaceStr {
						found = true
						break
					}
				}
				if !found {
					t.Error("It was expected to find at least on intel/facter/kernel metricType (but it wasn't there)")
				}
			})
		})
	})
}
