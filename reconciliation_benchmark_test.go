package main

import (
	"os"
	"testing"
)

// BenchmarkReconciliationReportParse records a local fixture baseline, not browser performance.
func BenchmarkReconciliationReportParse(b *testing.B) {
	for _, name := range []string{"success", "nested"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile("testdata/" + name + ".json")
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				report, err := parseReport(data)
				if err != nil {
					b.Fatal(err)
				}
				_ = flattenReport(report)
			}
		})
	}
}
