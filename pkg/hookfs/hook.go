/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package hookfs

import (
	"math/rand"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/ethercflow/hookfs/hookfs"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/sirupsen/logrus"
)

type ChaosbladeHookContext struct {
}

type ChaosbladeHook struct {
	MountPoint string
}

func (h *ChaosbladeHook) PreOpen(path string, flags uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "open")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostOpen(int32, hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreRead(path string, length int64, offset int64) ([]byte, bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "read")
	if err != nil {
		return nil, true, ctx, err
	}
	return nil, false, ctx, nil
}

func (h *ChaosbladeHook) PostRead(realRetCode int32, realBuf []byte, prehookCtx hookfs.HookContext) ([]byte, bool, error) {
	return nil, false, nil
}

func (h *ChaosbladeHook) PreWrite(path string, buf []byte, offset int64) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "write")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostWrite(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreMkdir(path string, mode uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "mkdir")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostMkdir(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreRmdir(path string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "rmdir")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostRmdir(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreOpenDir(path string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "opendir")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostOpenDir(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreFsync(path string, flags uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "fsync")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostFsync(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreFlush(path string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "flush")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostFlush(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreRelease(path string) (bool, hookfs.HookContext) {
	ctx := &ChaosbladeHookContext{}
	_ = h.doInjectFault(path, "release")
	return false, ctx
}

func (h *ChaosbladeHook) PostRelease(prehookCtx hookfs.HookContext) (hooked bool) {
	return false
}

func (h *ChaosbladeHook) PreTruncate(path string, size uint64) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "truncate")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostTruncate(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreGetAttr(path string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "getattr")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostGetAttr(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreChown(path string, uid uint32, gid uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "chown")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostChown(realRetCode int32, prehookCtx hookfs.HookContext) (hooked bool, err error) {
	return false, nil
}

func (h *ChaosbladeHook) PreChmod(path string, perms uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "chmod")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostChmod(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreUtimens(path string, atime *time.Time, mtime *time.Time) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "utimens")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostUtimens(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreAllocate(path string, off uint64, size uint64, mode uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "allocate")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostAllocate(realRetCode int32, prehookCtx hookfs.HookContext) (hooked bool, err error) {
	return false, nil
}

func (h *ChaosbladeHook) PreGetLk(path string, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "getlk")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostGetLk(realRetCode int32, prehookCtx hookfs.HookContext) (hooked bool, err error) {
	return false, nil
}

func (h *ChaosbladeHook) PreSetLk(path string, owner uint64, lk *fuse.FileLock, flags uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "setlk")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostSetLk(realRetCode int32, prehookCtx hookfs.HookContext) (hooked bool, err error) {
	return false, nil
}

func (h *ChaosbladeHook) PreSetLkw(path string, owner uint64, lk *fuse.FileLock, flags uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "setlkw")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostSetLkw(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreStatFs(path string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(path, "statfs")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostStatFs(prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreReadlink(name string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "readlink")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostReadlink(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreSymlink(value string, linkName string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(value, "symlink")
	if err != nil {
		return true, ctx, err
	}
	err = h.doInjectFault(linkName, "symlink")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostSymlink(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreCreate(name string, flags uint32, mode uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "create")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostCreate(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreAccess(name string, mode uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "access")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostAccess(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreLink(oldName string, newName string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(oldName, "link")
	if err != nil {
		return true, ctx, err
	}
	err = h.doInjectFault(newName, "link")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostLink(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreMknod(name string, mode uint32, dev uint32) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "mknod")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostMknod(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreRename(oldName string, newName string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(oldName, "rename")
	if err != nil {
		return true, ctx, err
	}
	err = h.doInjectFault(newName, "rename")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostRename(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreUnlink(name string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "unlink")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil

}
func (h *ChaosbladeHook) PostUnlink(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreGetXAttr(name string, attribute string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "getxattr")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostGetXAttr(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreListXAttr(name string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "listxattr")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostListXAttr(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreRemoveXAttr(name string, attr string) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "removexattr")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostRemoveXAttr(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) PreSetXAttr(name string, attr string, data []byte, flags int) (bool, hookfs.HookContext, error) {
	ctx := &ChaosbladeHookContext{}
	err := h.doInjectFault(name, "setxattr")
	if err != nil {
		return true, ctx, err
	}
	return false, ctx, nil
}

func (h *ChaosbladeHook) PostSetXAttr(realRetCode int32, prehookCtx hookfs.HookContext) (bool, error) {
	return false, nil
}

func (h *ChaosbladeHook) doInjectFault(relativePath, method string) error {
	logrus.WithFields(logrus.Fields{
		"method":       method,
		"relativePath": relativePath,
	}).Infoln("do Inject fault")
	val, ok := injectFaultCache.Load(method)
	if !ok || val == nil {
		return nil
	}
	faultMsg, ok := val.(*InjectMessage)
	if !ok {
		logrus.Errorf("convert to InjectMessage failed, %+v", val)
		return nil
	}
	logrus.WithField("faultMessage", faultMsg).Infoln("do Inject fault with inject message")
	if faultMsg.Path != "" {
		actualPath := path.Join(h.MountPoint, relativePath)
		if !strings.HasPrefix(actualPath, faultMsg.Path) {
			logrus.WithFields(logrus.Fields{
				"rulePath":   faultMsg.Path,
				"actualPath": actualPath,
			}).Infoln("the rule path does not contain the actual path")
			return nil
		}
	}
	if faultMsg.Percent > 0 && !probab(faultMsg.Percent) {
		return nil
	}
	var err error = nil
	if faultMsg.Errno != 0 {
		err = syscall.Errno(faultMsg.Errno)
	} else if faultMsg.Random {
		err = randomErrno()
	}
	if faultMsg.Delay > 0 {
		time.Sleep(time.Duration(faultMsg.Delay) * time.Millisecond)
	}
	return err

}

func randomErrno() error {
	// from E2BIG to EXFULL, notice linux only
	return syscall.Errno(rand.Intn(0x36-0x7) + 0x7)
}

func probab(percentage uint32) bool {
	return rand.Intn(99) < int(percentage)
}
