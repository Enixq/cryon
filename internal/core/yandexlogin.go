package core
import (
	"net/http"
	"sync"
)

// Вход в Yandex Music без официального способа выдачи токена. Мы используем
// implicit-grant OAuth того же публичного клиента, что и другие неофициальные
// клиенты Yandex Music: браузер открывается на странице авторизации, после
// логина Yandex редиректит на наш локальный адрес с токеном в hash-части URL
// (#access_token=...). Так как фрагмент не отправляется на сервер, отдаём
// маленькую HTML-страницу, которая читает hash в JS и постит токен обратно.
const (
	// yandexOAuthClientID — публичный client_id клиента Yandex Music,
	// используемый неофициальными библиотеками (implicit flow, response_type=token).
	yandexOAuthClientID = "23cabbbdc6cd418abb4b39c32c41195d"
	// yandexLoginAddr — локальный адрес для перехвата редиректа.
	yandexLoginAddr = "127.0.0.1:52178"
)

// yandexLogin инкапсулирует одноразовый локальный сервер захвата токена.
type yandexLogin struct {
	mu     sync.Mutex
	server *http.Server
}

// yandexCallbackHTML читает access_token из hash-части URL и отправляет его
// на локальный /token. Показывает результат пользователю.
const yandexCallbackHTML = `<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>Cryon — вход в Яндекс Музыку</title>
<style>
  body{margin:0;height:100vh;display:grid;place-items:center;background:#07080f;color:#e2e8f0;
       font-family:system-ui,Segoe UI,Roboto,sans-serif}
  .box{max-width:420px;text-align:center;padding:32px}
  h1{font-size:20px;margin:0 0 12px}
  p{color:#94a3b8;line-height:1.5}
  .ok{color:#34d399}.err{color:#f87171}
</style></head>
<body><div class="box">
  <h1>Cryon</h1>
  <p id="msg">Завершаем вход…</p>
</div>
<script>
(function(){
  var msg=document.getElementById("msg");
  var hash=window.location.hash.replace(/^#/,"");
  var params=new URLSearchParams(hash);
  var token=params.get("access_token");
  if(!token){
    msg.className="err";
    msg.textContent="Токен не найден. Закройте вкладку и попробуйте снова.";
    return;
  }
  fetch("/token",{method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded"},
    body:"access_token="+encodeURIComponent(token)})
    .then(function(r){
      if(r.ok){msg.className="ok";msg.textContent="Готово! Яндекс Музыка подключена. Можно вернуться в Cryon и закрыть эту вкладку.";}
      else{msg.className="err";msg.textContent="Не удалось сохранить токен. Попробуйте снова.";}
    })
    .catch(function(){msg.className="err";msg.textContent="Нет связи с приложением. Оно ещё запущено?";});
})();
</script>
</body></html>`
