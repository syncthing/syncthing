package main

import "fmt"

func MetricPrefix(n int64) string {
	if n > 1e9 {
		return fmt.Sprintf("%.02f G", float64(n)/1e9)
	}
	if n > 1e6 {
		return fmt.Sprintf("%.02f M", float64(n)/1e6)
	}
	if n > 1e3 {
		return fmt.Sprintf("%.01f k", float64(n)/1e3)
	}
	return fmt.Sprintf("%d ", n)
}

func BinaryPrefix(n int64) string {
	if n > 1<<30 {
		return fmt.Sprintf("%.02f Gi", float64(n)/(1<<30))
	}
	if n > 1<<20 {
		return fmt.Sprintf("%.02f Mi", float64(n)/(1<<20))
	}
	if n > 1<<10 {
		return fmt.Sprintf("%.01f Ki", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d ", n)
}
