//go:build cryonlan || cryonmobile

package core
import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// Рефлексивный RPC-мост, общий для LAN-сервера (lanserver.go, тег `cryonlan`)
// и встроенного сервера на устройстве (mobileserver.go, тег `cryonmobile`).
// Тег `cryonlan || cryonmobile` держит его вне обычной десктоп-сборки, поэтому
// «слепой» серверный код физически не влияет на рабочий Wails-билд.
//
// Контракт совпадает с тем, что Wails и так гоняет через JSON: вызываем
// ЭКСПОРТИРУЕМЫЙ метод App по имени, десериализуя аргументы из JSON в типы
// параметров и сериализуя ПЕРВОЕ не-error значение (как Wails-биндинг), а первый
// ненулевой error превращаем в HTTP-ошибку. Через reflection доступны только
// экспортируемые методы — startup/shutdown и приватные недостижимы.

type rpcRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

type rpcResponse struct {
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

func (a *App) serveRPC(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, "неверный запрос: "+err.Error(), http.StatusBadRequest)
		return
	}
	method := reflect.ValueOf(a).MethodByName(req.Method)
	if !method.IsValid() {
		writeRPCError(w, "неизвестный метод: "+req.Method, http.StatusNotFound)
		return
	}
	mt := method.Type()
	if mt.IsVariadic() {
		writeRPCError(w, "вариативные методы не поддерживаются: "+req.Method, http.StatusBadRequest)
		return
	}

	in := make([]reflect.Value, mt.NumIn())
	for i := 0; i < mt.NumIn(); i++ {
		pt := mt.In(i)
		argPtr := reflect.New(pt) // *pt — цель для json.Unmarshal
		if i < len(req.Args) && len(req.Args[i]) > 0 && string(req.Args[i]) != "null" {
			if err := json.Unmarshal(req.Args[i], argPtr.Interface()); err != nil {
				writeRPCError(w, fmt.Sprintf("аргумент %d метода %s: %v", i, req.Method, err), http.StatusBadRequest)
				return
			}
		}
		in[i] = argPtr.Elem()
	}

	out := method.Call(in)

	var result interface{}
	haveResult := false
	for _, v := range out {
		if v.Type().Implements(errorType) {
			if !v.IsNil() {
				writeRPCError(w, v.Interface().(error).Error(), http.StatusInternalServerError)
				return
			}
			continue
		}
		if !haveResult {
			result = v.Interface()
			haveResult = true
		}
	}
	writeJSON(w, rpcResponse{OK: true, Result: result})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeRPCError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(rpcResponse{OK: false, Error: msg})
}
