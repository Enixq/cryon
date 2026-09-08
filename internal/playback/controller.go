// Package playback управляет локальным воспроизведением через mpv.
//
// mpv запускается как отдельный процесс с JSON IPC (--input-ipc-server).
// Если mpv не найден в системе, контроллер переходит в режим "unavailable",
// и фронтенд воспроизводит поток самостоятельно через HTML5 <audio>.
package playback

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/logging"
)

// Backend — активный движок воспроизведения.
type Backend string

const (
	// BackendMPV — воспроизведение управляется процессом mpv.
	BackendMPV Backend = "mpv"
	// BackendNone — mpv недоступен, воспроизведение делегируется фронтенду.
	BackendNone Backend = "none"
)

// Status — снимок состояния плеера для фронтенда.
type Status struct {
	Backend   Backend `json:"backend"`   // "mpv" или "none"
	Available bool    `json:"available"` // доступно ли mpv-воспроизведение
	State     string  `json:"state"`     // idle | playing | paused
	TrackID   string  `json:"trackId,omitempty"`
	PositionS float64 `json:"positionS"` // текущая позиция, сек
	DurationS float64 `json:"durationS"` // полная длительность трека, сек (0 если ещё неизвестна)
	Volume    int     `json:"volume"`    // 0..100
	EOF       bool    `json:"eof"`       // достигнут конец трека
	Message   string  `json:"message,omitempty"`
}

// Controller управляет процессом mpv через IPC.
type Controller struct {
	mu sync.Mutex

	mpvPath string  // путь к mpv.exe; пусто, если не найден
	ipcPath string  // путь к IPC-сокету/пайпу
	backend Backend // текущий движок

	cmd     *exec.Cmd
	state   string
	trackID string
	volume  int
	// eqInner — внутренние звенья фильтра эквалайзера (equalizer=...,equalizer=...)
	// без обёртки lavfi[...] или "" (плоская АЧХ). Хранятся отдельно от
	// нормализации, чтобы собирать общий граф af из включённых звеньев.
	// Запоминается и повторно применяется при каждом новом Play: mpv стартует
	// новый процесс на каждый трек, и цепочка фильтров сбрасывается.
	eqInner string
	// normalize — включена ли нормализация громкости (dynaudnorm). Хранится
	// отдельно и компонуется с эквалайзером в один граф lavfi.
	normalize bool
}

var errUnavailable = errors.New("mpv недоступен")

// New создаёт контроллер, определяя доступность mpv.
// preferredPath — путь из конфига (env CRYON_MPV_PATH); может быть пустым.
func New(preferredPath string) *Controller {
	c := &Controller{
		state:   "idle",
		volume:  70,
		backend: BackendNone,
	}
	if p := findMPV(preferredPath); p != "" {
		c.mpvPath = p
		c.backend = BackendMPV
		c.ipcPath = defaultIPCPath()
		logging.L().Info("mpv найден, локальное воспроизведение доступно", "path", p)
	} else {
		logging.L().Info("mpv не найден — воспроизведение через фронтенд (HTML5 audio)")
	}
	return c
}

// Available сообщает, доступно ли воспроизведение через mpv.
func (c *Controller) Available() bool {
	return c.backend == BackendMPV
}

// Play запускает mpv на указанном URL/пути к файлу.
func (c *Controller) Play(trackID, streamURL string) error {
	if !c.Available() {
		return errUnavailable
	}
	if streamURL == "" {
		return errors.New("пустой URL потока")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopProcessLocked()

	_ = os.Remove(c.ipcPath)
	cmd := exec.Command(c.mpvPath,
		"--idle=yes",
		"--no-terminal",
		"--force-window=no",
		// keep-open=yes: по достижении конца трека mpv не выходит, а ставит
		// eof-reached=yes. Так фронтенд надёжно узнаёт об окончании и сам
		// переходит к следующему треку, а процесс остаётся готов к новому Play.
		"--keep-open=yes",
		"--input-ipc-server="+c.ipcPath,
		fmt.Sprintf("--volume=%d", c.volume),
		streamURL,
	)
	applyHideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запуск mpv не удался: %w", err)
	}

	if err := waitForIPC(c.ipcPath, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return fmt.Errorf("mpv IPC не поднялся: %w", err)
	}

	c.cmd = cmd
	c.trackID = trackID
	c.state = "playing"
	// Новый процесс mpv стартует без фильтров — возвращаем звуковой граф
	// (эквалайзер + нормализацию), если что-то включено. Ошибку не роняем:
	// без фильтров трек всё равно играет.
	if c.eqInner != "" || c.normalize {
		if err := c.applyAudioFilterLocked(); err != nil {
			logging.L().Warn("не удалось применить звуковые фильтры mpv", "err", err)
		}
	}
	return nil
}

// Pause ставит воспроизведение на паузу.
func (c *Controller) Pause() error {
	if !c.Available() {
		return errUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != "playing" {
		return nil
	}
	if err := c.sendCommandLocked("set_property", "pause", true); err != nil {
		return err
	}
	c.state = "paused"
	return nil
}

// Resume снимает с паузы.
func (c *Controller) Resume() error {
	if !c.Available() {
		return errUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != "paused" {
		return nil
	}
	if err := c.sendCommandLocked("set_property", "pause", false); err != nil {
		return err
	}
	c.state = "playing"
	return nil
}

// Stop останавливает воспроизведение и завершает процесс mpv.
func (c *Controller) Stop() error {
	if !c.Available() {
		return errUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopProcessLocked()
	c.state = "idle"
	c.trackID = ""
	return nil
}

// Seek перематывает на абсолютную позицию в секундах.
func (c *Controller) Seek(positionS float64) error {
	if !c.Available() {
		return errUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil {
		return errors.New("ничего не воспроизводится")
	}
	return c.sendCommandLocked("seek", positionS, "absolute")
}

// SetVolume задаёт громкость 0..100.
func (c *Controller) SetVolume(volume int) error {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.volume = volume
	if !c.Available() || c.cmd == nil {
		return nil
	}
	return c.sendCommandLocked("set_property", "volume", volume)
}

// eqFreqs — центральные частоты полос эквалайзера (Гц). Совпадают с полосами
// в UI и в Web Audio-фолбэке фронтенда, чтобы ползунки означали одно и то же
// независимо от того, каким движком играет трек.
var eqFreqs = []float64{60, 230, 910, 3600, 14000}

// SetEqualizer задаёт усиления полос эквалайзера (в дБ, по частотам eqFreqs).
// Пустой срез или все нули — эквалайзер снимается (плоская АЧХ). Значение
// запоминается и повторно применяется при каждом новом Play: mpv стартует
// новый процесс на трек и цепочку фильтров теряет.
func (c *Controller) SetEqualizer(gains []float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eqInner = buildEqInner(gains)
	if !c.Available() || c.cmd == nil {
		return nil
	}
	return c.applyAudioFilterLocked()
}

// SetNormalize включает/выключает нормализацию громкости (dynaudnorm).
// Как и эквалайзер, значение запоминается и восстанавливается на каждый Play.
func (c *Controller) SetNormalize(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalize = enabled
	if !c.Available() || c.cmd == nil {
		return nil
	}
	return c.applyAudioFilterLocked()
}

// applyAudioFilterLocked отправляет текущий звуковой граф mpv (нормализация +
// эквалайзер). Пустая строка снимает все фильтры. Вызывается под c.mu.
func (c *Controller) applyAudioFilterLocked() error {
	return c.sendCommandLocked("set_property", "af", c.composeAfLocked())
}

// composeAfLocked собирает единый граф lavfi из включённых звеньев: сначала
// нормализация (dynaudnorm выравнивает воспринимаемую громкость в реальном
// времени), затем эквалайзер. Пусто — фильтры сняты. Вызывается под c.mu.
func (c *Controller) composeAfLocked() string {
	parts := make([]string, 0, 2)
	if c.normalize {
		// Мягкая динамическая нормализация: f — длина кадра (мс), g — окно
		// сглаживания (кадры), m — макс. усиление, p — целевой пик. Значения
		// консервативные, чтобы не «дышать» на тихих участках.
		parts = append(parts, "dynaudnorm=f=250:g=15:p=0.9:m=10")
	}
	if c.eqInner != "" {
		parts = append(parts, c.eqInner)
	}
	if len(parts) == 0 {
		return ""
	}
	return "lavfi=[" + strings.Join(parts, ",") + "]"
}

// buildEqInner собирает звенья эквалайзера (без обёртки lavfi[...]) из усилений
// по полосам. Возвращает "" для плоской АЧХ (все нули) — тогда эквалайзер не
// добавляется в граф. Используется ffmpeg-фильтр equalizer (пиковый на каждой
// полосе), доступный в mpv через lavfi.
func buildEqInner(gains []float64) string {
	parts := make([]string, 0, len(eqFreqs))
	anyNonZero := false
	for i, f := range eqFreqs {
		g := 0.0
		if i < len(gains) {
			g = gains[i]
		}
		if g != 0 {
			anyNonZero = true
		}
		// t=o — ширина в октавах; w=1 — одна октава на полосу.
		parts = append(parts, fmt.Sprintf("equalizer=f=%g:t=o:w=1:g=%g", f, g))
	}
	if !anyNonZero {
		return ""
	}
	return strings.Join(parts, ",")
}

// Status возвращает текущее состояние плеера.
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	st := Status{
		Backend:   c.backend,
		Available: c.backend == BackendMPV,
		State:     c.state,
		TrackID:   c.trackID,
		Volume:    c.volume,
	}
	if !st.Available {
		st.Message = "mpv не установлен: воспроизведение через встроенный аудиоплеер"
		return st
	}
	if c.cmd != nil && c.state != "idle" {
		if pos, err := c.queryPositionLocked(); err == nil {
			st.PositionS = pos
		}
		if dur, err := c.queryDurationLocked(); err == nil {
			st.DurationS = dur
		}
		if eof, err := c.queryEOFLocked(); err == nil {
			st.EOF = eof
		}
	}
	return st
}

// Shutdown завершает mpv при закрытии приложения.
func (c *Controller) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopProcessLocked()
}

// stopProcessLocked завершает процесс mpv. Вызывается под c.mu.
func (c *Controller) stopProcessLocked() {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	_ = c.sendCommandLocked("quit")
	if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		logging.L().Warn("не удалось завершить mpv", "err", err)
	}
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
	if runtime.GOOS != "windows" {
		_ = os.Remove(c.ipcPath)
	}
}

// findMPV ищет mpv по предпочтительному пути, рядом с приложением и в PATH.
func findMPV(preferred string) string {
	var candidates []string
	if preferred != "" {
		candidates = append(candidates, preferred)
	}
	exeName := "mpv"
	if runtime.GOOS == "windows" {
		exeName = "mpv.exe"
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, exeName),
			filepath.Join(wd, "bin", exeName),
			filepath.Join(wd, "tools", exeName),
		)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\mpv\mpv.exe`,
			`C:\Program Files\MPV Player\mpv.exe`,
		)
	}
	candidates = append(candidates, exeName) // последним — поиск в PATH

	for _, cand := range candidates {
		if cand == exeName {
			if resolved, err := exec.LookPath(cand); err == nil {
				return resolved
			}
			continue
		}
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	return ""
}
