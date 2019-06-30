/**
 * uchroot.c
 * Based on super_chroot by Rich Felker
 * https://www.openwall.com/lists/musl/2013/08/01/2/1
 *
 * June 8th 2019 Eric Molitor <eric@molitor.org>
 **/

#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <unistd.h>
#include <signal.h>
#include <stdlib.h>
#include <fcntl.h>
#include <sys/mount.h>

int main(int argc, char **argv)
{
  uid_t uid = getuid();
  uid_t gid = getgid();

  unshare(CLONE_NEWUSER|CLONE_NEWNS);

  int fd = open("/proc/self/uid_map", O_RDWR);
  dprintf(fd, "%u %u 1\n", 0, uid);
  close(fd);

  fd = open("/proc/self/setgroups", O_RDWR);
  dprintf(fd, "%s\n", "deny");
  close(fd);

  fd = open("/proc/self/gid_map", O_RDWR);
  dprintf(fd, "%u %u 1\n", 0, gid);
  close(fd);

  chdir(argv[1]);
  mount("/dev", "./dev", 0, MS_BIND|MS_REC, 0);
  chroot(".");

  execv(argv[2], argv+2);
}
