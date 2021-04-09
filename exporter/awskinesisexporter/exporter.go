// Copyright 2019 OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package awskinesisexporter

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"
)

const (
	errInvalidContext = "invalid context"
)

// exporter implements an OpenTelemetry exporter that pushes OpenTelemetry data to AWS Kinesis
type Exporter struct {
	producer   producer
	logger     *zap.Logger
	marshaller Marshaller
}

var _ component.TracesExporter = (*Exporter)(nil)
var _ component.MetricsExporter = (*Exporter)(nil)
// newExporter creates a new exporter with the passed in configurations.
// It starts the AWS session and setups the relevant connections.
func newExporter(c *Config, logger *zap.Logger) (*Exporter, error) {
	// Get marshaller based on config
	marshaller := defaultMarshallers()[c.Encoding]
	if marshaller == nil {
		return nil, fmt.Errorf("unrecognized encoding")
	}

	pr, err := newKinesisProducer(c, logger)
	if err != nil {
		return nil, err
	}

	return &Exporter{producer: pr, marshaller: marshaller, logger: logger}, nil
}

// start tells the exporter to start. The exporter may prepare for exporting
// by connecting to the endpoint. Host parameter can be used for communicating
// with the host after start() has already returned. If error is returned by
// start() then the collector startup will be aborted.
func (e *Exporter) Start(ctx context.Context, _ component.Host) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf(errInvalidContext)
	}

	e.producer.start()
	return nil
}

// Capabilities implements the consumer interface.
func (e Exporter) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// Shutdown is invoked during exporter shutdown.
func (e Exporter) Shutdown(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf(errInvalidContext)
	}

	e.producer.stop()
	return nil
}

// ConsumeTraces receives a span batch and exports it to AWS Kinesis
func (e Exporter) ConsumeTraces(ctx context.Context, td pdata.Traces) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf(errInvalidContext)
	}

	pBatches, err := e.marshaller.MarshalTraces(td)
	if err != nil {
		e.logger.Error("error translating span batch", zap.Error(err))
		return consumererror.Permanent(err)
	}

	if err = e.producer.put(pBatches, uuid.New().String()); err != nil {
		tenants := traceTenants(td)
		e.logger.Error("error exporting span to kinesis", zap.Error(err), zap.Strings("services", tenants))
		return err
	}

	return nil
}

func (e Exporter) ConsumeMetrics(ctx context.Context, md pdata.Metrics) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf(errInvalidContext)
	}

	pBatches, err := e.marshaller.MarshalMetrics(md)
	if err != nil {
		e.logger.Error("error translating metrics batch", zap.Error(err))
		return consumererror.Permanent(err)
	}

	if err = e.producer.put(pBatches, uuid.New().String()); err != nil {
		e.logger.Error("error exporting metrics to kinesis", zap.Error(err))
		return err
	}

	return nil
}
