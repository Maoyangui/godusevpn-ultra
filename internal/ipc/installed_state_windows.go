//go:build windows

package ipc

func (p *windowsPeerSession) installedPID() (uint32, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.foreground || p.pid == 0 {
		return 0, authError("当前连接不是安装服务")
	}
	return p.pid, nil
}
