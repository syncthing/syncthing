// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

const namePrefix = "syncthing_usage_"

type metricsSet struct {
	srv *server

	gauges         map[string]prometheus.Gauge
	gaugeVecs      map[string]*prometheus.GaugeVec
	gaugeVecLabels map[string][]string
	summaries      map[string]*metricSummary

	collectMut    sync.RWMutex
	collectCutoff time.Duration
}

func newMetricsSet(srv *server) *metricsSet {
	s := &metricsSet{
		srv:            srv,
		gauges:         make(map[string]prometheus.Gauge),
		gaugeVecs:      make(map[string]*prometheus.GaugeVec),
		gaugeVecLabels: make(map[string][]string),
		summaries:      make(map[string]*metricSummary),
		collectCutoff:  -24 * time.Hour,
	}

	var initForType func(reflect.Type)
	initForType = func(t reflect.Type) {
		for i := range t.NumField() {
			field := t.Field(i)
			if field.Type.Kind() == reflect.Struct {
				initForType(field.Type)
				continue
			}
			name, typ, label := fieldNameTypeLabel(field)
			sname, labels := nameConstLabels(name)
			switch typ {
			case "gauge":
				s.gauges[name] = prometheus.NewGauge(prometheus.GaugeOpts{
					Name:        namePrefix + sname,
					ConstLabels: labels,
				})
			case "summary":
				s.summaries[name] = newMetricSummary(namePrefix+sname, nil, labels)
			case "gaugeVec":
				s.gaugeVecLabels[name] = append(s.gaugeVecLabels[name], label)
			case "summaryVec":
				s.summaries[name] = newMetricSummary(namePrefix+sname, []string{label}, labels)
			}
		}
	}
	initForType(reflect.ValueOf(contract.Report{}).Type())

	for name, labels := range s.gaugeVecLabels {
		s.gaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: namePrefix + name,
		}, labels)
	}

	return s
}

func fieldNameTypeLabel(rf reflect.StructField) (string, string, string) {
	metric := rf.Tag.Get("metric")
	name, typ, ok := strings.Cut(metric, ",")
	if !ok {
		return "", "", ""
	}
	gv, label, ok := strings.Cut(typ, ":")
	if ok {
		typ = gv
	}
	return name, typ, label
}

func nameConstLabels(name string) (string, prometheus.Labels) {
	if name == "-" {
		return "", nil
	}
	name, labels, ok := strings.Cut(name, "{")
	if !ok {
		return name, nil
	}
	lls := strings.Split(labels[:len(labels)-1], ",")
	m := make(map[string]string)
	for _, l := range lls {
		k, v, _ := strings.Cut(l, "=")
		m[k] = v
	}
	return name, m
}

func (s *metricsSet) Serve(ctx context.Context) error {
	s.recalc()

	const recalcInterval = 5 * time.Minute
	next := time.Until(time.Now().Truncate(recalcInterval).Add(recalcInterval))
	recalcTimer := time.NewTimer(next)
	defer recalcTimer.Stop()

	for {
		select {
		case <-recalcTimer.C:
			s.recalc()
			next := time.Until(time.Now().Truncate(recalcInterval).Add(recalcInterval))
			recalcTimer.Reset(next)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *metricsSet) recalc() {
	s.collectMut.Lock()
	defer s.collectMut.Unlock()

	t0 := time.Now()
	defer func() {
		dur := time.Since(t0)
		slog.Info("Metrics recalculated", "d", dur.String())
		metricsRecalcSecondsLast.Set(dur.Seconds())
		metricsRecalcSecondsTotal.Add(dur.Seconds())
		metricsRecalcsTotal.Inc()
	}()

	for _, g := range s.gauges {
		g.Set(0)
	}
	for _, g := range s.gaugeVecs {
		g.Reset()
	}
	for _, g := range s.summaries {
		g.Reset()
	}

	cutoff := time.Now().Add(s.collectCutoff)
	s.srv.reports.Range(func(key string, r *contract.Report) bool {
		if s.collectCutoff < 0 && r.Received.Before(cutoff) {
			s.srv.reports.Delete(key)
			return true
		}
		s.addReport(r)
		return true
	})
}

func (s *metricsSet) addReport(r *contract.Report) {
	gaugeVecs := make(map[string][]string)
	s.addReportStruct(reflect.ValueOf(r).Elem(), gaugeVecs)
	for name, lv := range gaugeVecs {
		s.gaugeVecs[name].WithLabelValues(lv...).Add(1)
	}
}

func (s *metricsSet) addReportStruct(v reflect.Value, gaugeVecs map[string][]string) {
	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		if field.Kind() == reflect.Struct {
			s.addReportStruct(field, gaugeVecs)
			continue
		}

		name, typ, label := fieldNameTypeLabel(t.Field(i))
		switch typ {
		case "gauge":
			switch v := field.Interface().(type) {
			case int:
				s.gauges[name].Add(float64(v))
			case string:
				s.gaugeVecs[name].WithLabelValues(v).Add(1)
			case bool:
				if v {
					s.gauges[name].Add(1)
				}
			}
		case "gaugeVec":
			var labelValue string
			switch v := field.Interface().(type) {
			case string:
				labelValue = v
			case int:
				labelValue = strconv.Itoa(v)
			case map[string]int:
				for k, v := range v {
					labelValue = k
					field.SetInt(int64(v))
					break
				}
			}
			if _, ok := gaugeVecs[name]; !ok {
				gaugeVecs[name] = make([]string, len(s.gaugeVecLabels[name]))
			}
			for i, l := range s.gaugeVecLabels[name] {
				if l == label {
					gaugeVecs[name][i] = labelValue
					break
				}
			}
		case "summary", "summaryVec":
			switch v := field.Interface().(type) {
			case int:
				s.summaries[name].Observe("", float64(v))
			case float64:
				s.summaries[name].Observe("", v)
			case []int:
				for _, v := range v {
					s.summaries[name].Observe("", float64(v))
				}
			case map[string]int:
				for k, v := range v {
					if k == "" {
						// avoid empty string labels as those are the sign
						// of a non-vec summary
						k = "unknown"
					}
					s.summaries[name].Observe(k, float64(v))
				}
			}
		}
	}
}

func (s *metricsSet) Describe(c chan<- *prometheus.Desc) {
	for _, g := range s.gauges {
		g.Describe(c)
	}
	for _, g := range s.gaugeVecs {
		g.Describe(c)
	}
	for _, g := range s.summaries {
		g.Describe(c)
	}
}

func (s *metricsSet) Collect(c chan<- prometheus.Metric) {
	s.collectMut.RLock()
	defer s.collectMut.RUnlock()

	t0 := time.Now()
	defer func() {
		dur := time.Since(t0).Seconds()
		metricsCollectSecondsLast.Set(dur)
		metricsCollectSecondsTotal.Add(dur)
		metricsCollectsTotal.Inc()
	}()

	for _, g := range s.gauges {
		c <- g
	}
	for _, g := range s.gaugeVecs {
		g.Collect(c)
	}
	for _, g := range s.summaries {
		g.Collect(c)
	}
}

type metricSummary struct {
	name   string
	values map[string][]float64
	zeroes map[string]int

	qDesc     *prometheus.Desc
	countDesc *prometheus.Desc
	sumDesc   *prometheus.Desc
	zDesc     *prometheus.Desc
}

func newMetricSummary(name string, labels []string, constLabels prometheus.Labels) *metricSummary {
	return &metricSummary{
		name:      name,
		values:    make(map[string][]float64),
		zeroes:    make(map[string]int),
		qDesc:     prometheus.NewDesc(name, "", append(labels, "quantile"), constLabels),
		countDesc: prometheus.NewDesc(name+"_nonzero_count", "", labels, constLabels),
		sumDesc:   prometheus.NewDesc(name+"_sum", "", labels, constLabels),
		zDesc:     prometheus.NewDesc(name+"_zero_count", "", labels, constLabels),
	}
}

func (q *metricSummary) Observe(labelValue string, v float64) {
	if v == 0 {
		q.zeroes[labelValue]++
		return
	}
	q.values[labelValue] = append(q.values[labelValue], v)
}

func (q *metricSummary) Describe(c chan<- *prometheus.Desc) {
	c <- q.qDesc
	c <- q.countDesc
	c <- q.sumDesc
	c <- q.zDesc
}

func (q *metricSummary) Collect(c chan<- prometheus.Metric) {
	for lv, vs := range q.values {
		var labelVals []string
		if lv != "" {
			labelVals = []string{lv}
		}

		c <- prometheus.MustNewConstMetric(q.countDesc, prometheus.GaugeValue, float64(len(vs)), labelVals...)
		c <- prometheus.MustNewConstMetric(q.zDesc, prometheus.GaugeValue, float64(q.zeroes[lv]), labelVals...)

		var sum float64
		for _, v := range vs {
			sum += v
		}
		c <- prometheus.MustNewConstMetric(q.sumDesc, prometheus.GaugeValue, sum, labelVals...)

		if len(vs) == 0 {
			return
		}

		slices.Sort(vs)

		pctiles := []float64{0, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.95, 0.975, 0.99, 1}
		for _, pct := range pctiles {
			idx := int(float64(len(vs)-1) * pct)
			c <- prometheus.MustNewConstMetric(q.qDesc, prometheus.GaugeValue, vs[idx], append(labelVals, fmt.Sprint(pct))...)
		}
	}
}

func (q *metricSummary) Reset() {
	clear(q.values)
	clear(q.zeroes)
}
