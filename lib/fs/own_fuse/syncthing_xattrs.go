package own_fuse

import "golang.org/x/sys/unix"

// "hash_computing" is set when computation of the files hash starts.
// When computation is done, "hash" is set to the result.
// After this, its checked if "hash_computing" is still there.
// If yes, then its guaranteed that the files data was not changed
// during the period of computation, and "hash_computing" is removed.
// If no, then there was a change, and the hash needs to be re-computed,
// by re-starting the whole procedure.
const attr_to_delete_1 string = "user.syncthing.hash_computing"
const attr_to_delete_2 string = "user.syncthing.hash"

func willBeChanged(path string) {
	unix.Lremovexattr(path, attr_to_delete_1)
	unix.Lremovexattr(path, attr_to_delete_2)
}

func willBeChangedFd(fd int) {
	unix.Fremovexattr(fd, attr_to_delete_1)
	unix.Fremovexattr(fd, attr_to_delete_2)
}
