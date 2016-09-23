/**********************************************************\
|                                                          |
|                          hprose                          |
|                                                          |
| Official WebSite: http://www.hprose.com/                 |
|                   http://www.hprose.org/                 |
|                                                          |
\**********************************************************/
/**********************************************************\
 *                                                        *
 * rpc/fasthttp_service.go                                *
 *                                                        *
 * hprose fasthttp service for Go.                        *
 *                                                        *
 * LastModified: Sep 23, 2016                             *
 * Author: Ma Bingyao <andot@hprose.com>                  *
 *                                                        *
\**********************************************************/

package rpc

import (
	"reflect"
	"strings"
	"unsafe"

	"github.com/hprose/hprose-golang/util"
	"github.com/valyala/fasthttp"
)

// FastHTTPContext is the hprose fasthttp context
type FastHTTPContext struct {
	*ServiceContext
	RequestCtx *fasthttp.RequestCtx
}

// NewFastHTTPContext is the constructor of FastHTTPContext
func NewFastHTTPContext(
	clients Clients, ctx *fasthttp.RequestCtx) (context *FastHTTPContext) {
	context = new(FastHTTPContext)
	context.ServiceContext = NewServiceContext(clients)
	context.ServiceContext.TransportContext = context
	context.RequestCtx = ctx
	return
}

// FastHTTPService is the hprose fasthttp service
type FastHTTPService struct {
	baseHTTPService
}

type fastSendHeaderEvent interface {
	OnSendHeader(context *FastHTTPContext)
}

type fastSendHeaderEvent2 interface {
	OnSendHeader(context *FastHTTPContext) error
}

func fasthttpFixArguments(args []reflect.Value, context *ServiceContext) {
	i := len(args) - 1
	switch args[i].Type() {
	case fasthttpContextType:
		if c, ok := context.TransportContext.(*FastHTTPContext); ok {
			args[i] = reflect.ValueOf(c)
		}
	case fasthttpRequestCtxType:
		if c, ok := context.TransportContext.(*FastHTTPContext); ok {
			args[i] = reflect.ValueOf(c.RequestCtx)
		}
	default:
		DefaultFixArguments(args, context)
	}
}

// NewFastHTTPService is the constructor of FastHTTPService
func NewFastHTTPService() (service *FastHTTPService) {
	service = (*FastHTTPService)(unsafe.Pointer(newBaseHTTPService()))
	service.FixArguments = fasthttpFixArguments
	return
}

func (service *FastHTTPService) xmlFileHandler(
	ctx *fasthttp.RequestCtx, path string, context []byte) bool {
	requestPath := util.ByteString(ctx.Path())
	if context == nil || strings.ToLower(requestPath) != path {
		return false
	}
	ifModifiedSince := util.ByteString(ctx.Request.Header.Peek("if-modified-since"))
	ifNoneMatch := util.ByteString(ctx.Request.Header.Peek("if-none-match"))
	if ifModifiedSince == service.lastModified && ifNoneMatch == service.etag {
		ctx.SetStatusCode(304)
	} else {
		contentLength := len(context)
		ctx.Response.Header.Set("Last-Modified", service.lastModified)
		ctx.Response.Header.Set("Etag", service.etag)
		ctx.Response.Header.SetContentType("text/xml")
		ctx.Response.Header.Set("Content-Length", util.Itoa(contentLength))
		ctx.SetBody(context)
	}
	return true
}

func (service *FastHTTPService) crossDomainXMLHandler(
	ctx *fasthttp.RequestCtx) bool {
	path := "/crossdomain.xml"
	context := service.crossDomainXMLContent
	return service.xmlFileHandler(ctx, path, context)
}

func (service *FastHTTPService) clientAccessPolicyXMLHandler(
	ctx *fasthttp.RequestCtx) bool {
	path := "/clientaccesspolicy.xml"
	context := service.clientAccessPolicyXMLContent
	return service.xmlFileHandler(ctx, path, context)
}

func (service *FastHTTPService) fireSendHeaderEvent(
	context *FastHTTPContext) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = NewPanicError(e)
		}
	}()
	switch event := service.Event.(type) {
	case fastSendHeaderEvent:
		event.OnSendHeader(context)
	case fastSendHeaderEvent2:
		err = event.OnSendHeader(context)
	}
	return err
}

func (service *FastHTTPService) sendHeader(
	context *FastHTTPContext) (err error) {
	if err = service.fireSendHeaderEvent(context); err != nil {
		return err
	}
	ctx := context.RequestCtx
	ctx.Response.Header.Set("Content-Type", "text/plain")
	if service.P3P {
		ctx.Response.Header.Set("P3P",
			`CP="CAO DSP COR CUR ADM DEV TAI PSA PSD IVAi IVDi `+
				`CONi TELo OTPi OUR DELi SAMi OTRi UNRi PUBi IND PHY ONL `+
				`UNI PUR FIN COM NAV INT DEM CNT STA POL HEA PRE GOV"`)
	}
	if service.CrossDomain {
		origin := util.ByteString(ctx.Request.Header.Peek("origin"))
		if origin != "" && origin != "null" {
			if len(service.accessControlAllowOrigins) == 0 ||
				service.accessControlAllowOrigins[origin] {
				ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
				ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
			}
		} else {
			ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		}
	}
	return nil
}

// ServeFastHTTP is the hprose fasthttp handler method
func (service *FastHTTPService) ServeFastHTTP(ctx *fasthttp.RequestCtx) {
	if service.clientAccessPolicyXMLHandler(ctx) ||
		service.crossDomainXMLHandler(ctx) {
		return
	}
	context := NewFastHTTPContext(service, ctx)
	var resp []byte
	if err := service.sendHeader(context); err == nil {
		switch util.ByteString(ctx.Method()) {
		case "GET":
			if service.GET {
				resp = service.doFunctionList(context.ServiceContext)
			} else {
				ctx.SetStatusCode(403)
			}
		case "POST":
			resp = service.Handle(ctx.Request.Body(), context.ServiceContext)
		}
	} else {
		resp = service.endError(err, context.ServiceContext)
	}
	context.RequestCtx = nil
	ctx.Response.Header.Set("Content-Length", util.Itoa(len(resp)))
	ctx.SetBody(resp)
}
