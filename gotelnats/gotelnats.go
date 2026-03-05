// Package gotelnats provides convenience functions for opentelemetry.
package gotelnats

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/cyverse-de/p/go/header"
	"github.com/cyverse-de/p/go/svcerror"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

const DefaultTLSCertPath = "/etc/nats/tls/tls.crt"
const DefaultTLSKeyPath = "/etc/nats/tls/tls.key"
const DefaultTLSCAPath = "/etc/nats/tls/ca.crt"
const DefaultCredsPath = "/etc/nats/creds/services.creds"
const DefaultMaxReconnects = 10
const DefaultReconnectWait = 1

type PBTextMapCarrier struct {
	*header.Header
}

func (p *PBTextMapCarrier) Get(key string) string {
	v, ok := p.Map[key]
	if !ok {
		return ""
	}
	if len(v.Value) == 0 {
		return ""
	}
	return v.Value[0]
}

func (p *PBTextMapCarrier) Set(key string, value string) {
	p.Map[key] = &header.Header_Value{
		Value: []string{
			value,
		},
	}
}

func (p *PBTextMapCarrier) Keys() []string {
	var keys []string
	for k := range p.Map {
		keys = append(keys, k)
	}
	return keys
}

const natsName = "nats"
const tracerModule = "github.com/cyverse-de/go-mod/gotelnats"

type Operation int

const (
	Process Operation = iota
	Send
)

func (o Operation) String() string {
	switch o {
	case Process:
		return "process"
	case Send:
		return "send"
	}
	return "unknown"
}

// StartSpan creates a new context and populate it with information from
// the carrier. You will need to call defer span.End() in the calling code.
// Returns a new context based on context.Background() that can be passed to
// InjectSpan().
func StartSpan(carrier propagation.TextMapCarrier, subject string, op Operation) (context.Context, trace.Span) {
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	tracer := otel.GetTracerProvider().Tracer(tracerModule)
	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", subject, op.String()), trace.WithSpanKind(trace.SpanKindConsumer))

	span.SetAttributes(
		semconv.MessagingSystemKey.String(natsName),
		semconv.MessagingProtocolKey.String(natsName),
		semconv.MessagingProtocolVersionKey.String(nats.Version),
		semconv.MessagingDestinationKindTopic,
		semconv.MessagingDestinationKey.String(subject),
	)

	if op != Send {
		span.SetAttributes(
			semconv.MessagingOperationKey.String(op.String()),
		)
	}

	return ctx, span
}

// InjectSpan adds information from the context to the carrier so that it can
// be serialized and sent along to other services. You will need to call
// 'defer span.End()' in the calling code. Returns a context based on the one
// passed into the function, plus a new span.
func InjectSpan(ctx context.Context, carrier propagation.TextMapCarrier, subject string, op Operation) (context.Context, trace.Span) {
	var tracer trace.Tracer

	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		tracer = span.TracerProvider().Tracer(tracerModule)
	} else {
		tracer = otel.GetTracerProvider().Tracer(tracerModule)
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", subject, op.String()), trace.WithSpanKind(trace.SpanKindProducer))

	span.SetAttributes(
		semconv.MessagingSystemKey.String(natsName),
		semconv.MessagingProtocolKey.String(natsName),
		semconv.MessagingProtocolVersionKey.String(nats.Version),
		semconv.MessagingDestinationKindTopic,
		semconv.MessagingDestinationKey.String(subject),
	)

	if op != Send {
		span.SetAttributes(
			semconv.MessagingOperationKey.String(op.String()),
		)
	}

	otel.GetTextMapPropagator().Inject(ctx, carrier)

	return ctx, span
}

// DEServiceError implements Go's error interface around a
// *svcerror.ServiceError.
type DEServiceError struct {
	ServiceError *svcerror.ServiceError
}

// NewDEServiceError returns a newly created instance of DEServiceError. The
// httpStatusCode parameters after 'message' are variadic, but only the first
// value is used. No error is raised if you pass in multiple status codes, the
// extra ones are just ignored. This prevents us from having to include error
// handling logic inside our error handling logic, which just gets annoying.
func NewDEServiceError(errorCode svcerror.ErrorCode, message string, httpStatusCode ...int32) *DEServiceError {
	se := svcerror.ServiceError{
		ErrorCode: errorCode,
		Message:   message,
	}

	if len(httpStatusCode) > 0 {
		se.StatusCode = httpStatusCode[0]
	}

	return &DEServiceError{
		ServiceError: &se,
	}
}

func (d DEServiceError) Error() string {
	return d.ServiceError.Message
}

// ErrorOptions contains values that can be set in a *svcerror.ServiceError
// returned by InitServiceError().
type ErrorOptions struct {
	ErrorCode  svcerror.ErrorCode
	StatusCode int32 // HTTP status code
}

// InitServiceError creates and returns a new *svcerror.ServiceError based on
// the parameters and options passed in. It also adds error information to the
// span recorded in the context that gets passed in. See ErrorOptions for the
// supported options.
//
// The error that is passed in can be either a normal error or a DEServiceError
// (or *DEServiceError). You can't pass in a ServiceError since it doesn't
// implement Go's error interface.
//
// Nothing goes out over the wire when calling this function. Use the return
// value of this function to set the Error field of a DEResponse.
func InitServiceError(ctx context.Context, err error, opts *ErrorOptions) *svcerror.ServiceError {
	var svcErr *svcerror.ServiceError

	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	switch e := err.(type) {
	case DEServiceError:
		svcErr = e.ServiceError
	case *DEServiceError:
		svcErr = e.ServiceError
	default:
		svcErr = &svcerror.ServiceError{
			ErrorCode: svcerror.ErrorCode_UNSPECIFIED,
			Message:   e.Error(),
		}
	}

	if opts != nil {
		if opts.ErrorCode != svcerror.ErrorCode_UNSET {
			svcErr.ErrorCode = opts.ErrorCode
		}
		if opts.StatusCode != 0 {
			svcErr.StatusCode = opts.StatusCode
		}
	}

	return svcErr
}

// DERequest is a constraint interface that says that types must include a
// Header implementation an be a protocol buffer message type.
type DERequest interface {
	GetHeader() *header.Header

	proto.Message
}

// DEResponse is a constraint interface that says that types must include a
// Header and ServiceError implementation and be a protocol buffer message type.
type DEResponse interface {
	GetHeader() *header.Header
	GetError() *svcerror.ServiceError

	proto.Message
}

// NewHeader returns a newly created *header.Header with the Map field
// initialized.
func NewHeader() *header.Header {
	return &header.Header{
		Map: make(map[string]*header.Header_Value),
	}
}

// Request handles instrumenting the outgoing request with telemetry info,
// blocking until the request is responded to, and handling responses containing
// errors returned by the other service.
//
// Handles turning a svcerror.ServiceError embedded in the response into a
// DEServiceError, which implements Go's error interface and can be passed
// around like a normal error.
//
// It is a generic function that can accept requests implementing the DERequest
// interface and responses implementing the DEResponse interface.
func Request[ReqType DERequest, RespType DEResponse](
	ctx context.Context,
	conn *nats.EncodedConn,
	subject string,
	request ReqType,
	response RespType,
) error {
	var err error

	carrier := PBTextMapCarrier{
		Header: request.GetHeader(),
	}

	_, span := InjectSpan(ctx, &carrier, subject, Send)
	defer span.End()

	// Uses the EncodedCode to unmarshal the data into dePtr.
	err = conn.Request(subject, request, response, 30*time.Second)
	if err != nil {
		return err
	}

	// Since the potential error details are embedded in the unmarshalled
	// data, we have to look to make sure it's not set. Believe it or not, this
	// is far easier to deal with than sending out separate service error
	// messages.
	respErr := response.GetError()
	if respErr != nil {
		if respErr.ErrorCode != svcerror.ErrorCode_UNSET {
			if respErr.StatusCode != 0 { // httpStatusCode is optional.
				err = NewDEServiceError(respErr.ErrorCode, respErr.Message, respErr.StatusCode)
			} else {
				err = NewDEServiceError(respErr.ErrorCode, respErr.Message)
			}
			return err
		}
	}

	return nil
}

// Publish instruments outgoing responses with telemetry information.
// Does not expect a response.
func Publish[ReqType DERequest](ctx context.Context, conn *nats.EncodedConn, subject string, request ReqType) error {
	var err error

	carrier := PBTextMapCarrier{
		Header: request.GetHeader(),
	}

	_, span := InjectSpan(ctx, &carrier, subject, Send)
	defer span.End()

	if err = conn.Publish(subject, request); err != nil {
		return err
	}

	return nil
}

// PublishResponse instruments outgoing responses with telemetry information.
// It is a generic function that will accept responses implementing the
// DEResponse interface. The response should be a pointer to a concrete
// implementation of the DEResponse interface.
//
// The response type has an *svcerror.ServiceError field embedded in it, so any
// errors that crop up during creation of the response should be set there
// before the value is passed into this function. See the InitServiceError()
// function for more info.
func PublishResponse[ResponseT DEResponse](
	ctx context.Context,
	conn *nats.EncodedConn,
	reply string,
	response ResponseT,
) error {
	reflectValue := reflect.ValueOf(response)
	if reflectValue.Kind() != reflect.Pointer || reflectValue.IsNil() {
		return fmt.Errorf("cannot unmarshal into type '%s'; it must be a pointer and non-nil", reflect.TypeOf(response))
	}

	carrier := PBTextMapCarrier{
		Header: response.GetHeader(),
	}

	_, span := InjectSpan(ctx, &carrier, reply, Send)
	defer span.End()

	return conn.Publish(reply, response)
}
