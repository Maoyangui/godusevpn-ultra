package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"time"
)

// InstalledState binds one fresh GetState response to the authenticated,
// currently running SCM service process on that same held pipe connection.
// It never admits a foreground peer, even when ordinary Call would admit it.
type InstalledState struct {
	ServicePID uint32
	View       StateView
}

type installedPeer interface {
	installedPID() (uint32, error)
}

func ReadInstalledState(ctx context.Context) (InstalledState, error) {
	return readInstalledStateWithDial(ctx, dial)
}

func readInstalledStateWithDial(ctx context.Context, connect func(context.Context) (net.Conn, error)) (InstalledState, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	conn, err := connect(ctx)
	if err != nil {
		return InstalledState{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	auth, ok := conn.(*authenticatedConn)
	if !ok || auth.peer == nil {
		return InstalledState{}, authError("只读浏览器门禁要求已认证的安装服务")
	}
	peer, ok := auth.peer.(installedPeer)
	if !ok {
		return InstalledState{}, authError("无法核对安装服务实例")
	}
	pid, err := peer.installedPID()
	if err != nil || pid == 0 {
		return InstalledState{}, authError("不允许前台或未知服务实例")
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return InstalledState{}, err
	}
	req := Request{ID: time.Now().UnixNano(), Method: MGetState}
	data, _ := json.Marshal(req)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return InstalledState{}, err
	}
	line, err := bufio.NewReader(io.LimitReader(conn, 1<<20)).ReadBytes('\n')
	if err != nil {
		return InstalledState{}, err
	}
	var response Response
	if err := json.Unmarshal(line, &response); err != nil {
		return InstalledState{}, err
	}
	if response.ID != req.ID {
		return InstalledState{}, ErrResponseMismatch
	}
	if !response.OK {
		return InstalledState{}, &CallError{Code: response.Code, Msg: response.Error}
	}
	var view StateView
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return InstalledState{}, ErrResponseMismatch
	}
	if err := json.Unmarshal(response.Result, &view); err != nil {
		return InstalledState{}, err
	}
	currentPID, err := peer.installedPID()
	if err != nil || currentPID != pid || ctx.Err() != nil {
		return InstalledState{}, authError("响应后安装服务实例变化或无法核对")
	}
	return InstalledState{ServicePID: pid, View: view}, nil
}
