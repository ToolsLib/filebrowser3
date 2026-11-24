package fbhttp

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startCommand(cmd *exec.Cmd) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	stop := func() { _ = windows.CloseHandle(job) }
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		stop()
		return nil, err
	}
	// Start suspended so children cannot escape assignment to the job.
	// cmd.Wait finishes CommandContext's cancellation goroutine before stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Cancel = func() error { return windows.TerminateJobObject(job, 1) }
	if err := cmd.Start(); err != nil {
		stop()
		return nil, err
	}
	if err := attachCommandJob(job, cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		stop()
		return nil, err
	}
	return stop, nil
}

func attachCommandJob(job windows.Handle, cmd *exec.Cmd) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(cmd.Process.Pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		return err
	}
	return fmt.Errorf("could not find command's initial thread")
}
