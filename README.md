## acbup

Oh no, I think I wrote a very basic version of git, but for making versioned backups:

A directory is backed up by recursively walking it, hashing each file, and storing the contents under a path
that corresponds to the hash.

Then a refs file is created which maps filenames to hashes. Once complete, it too is hashed and stored under a
path that corresponds to it's hash.

Here's an example of it running a test (via earthly):

    ./tests+test-bkup | --> COPY ..+acbup/acbup /bin/.
    ./tests+test-bkup | --> RUN echo "src=/root/files" > acbup.conf && echo "dst=/root/bkup" >> acbup.conf
    ./tests+test-bkup | --> RUN mkdir /root/files
    ./tests+test-bkup | --> RUN echo "alpha" > /root/files/a.txt
    ./tests+test-bkup | --> RUN echo "bravo" > /root/files/b.txt
    ./tests+test-bkup | --> RUN echo "charlie" > /root/files/c.txt
    ./tests+test-bkup | --> RUN echo "delta" > /root/files/d.txt
    ./tests+test-bkup | --> RUN mkdir -p /root/files/sub/dir
    ./tests+test-bkup | --> RUN echo "echo" > /root/files/sub/dir/e.txt
    ./tests+test-bkup | --> RUN acbup --config=acbup.conf
    ./tests+test-bkup | "/root/files/a.txt" -> "d046cd9b7ffb7661e449683313d41f6fc33e3130"; /root/bkup/data/d0/46/cd/d046cd9b7ffb7661e449683313d41f6fc33e3130 backing up
    ./tests+test-bkup | "/root/files/b.txt" -> "bb596efe9e3023a502013767a0559a94a5eea4bc"; /root/bkup/data/bb/59/6e/bb596efe9e3023a502013767a0559a94a5eea4bc backing up
    ./tests+test-bkup | "/root/files/c.txt" -> "d6ed21679f692a68a2202cb9a2ff1e861f97fc63"; /root/bkup/data/d6/ed/21/d6ed21679f692a68a2202cb9a2ff1e861f97fc63 backing up
    ./tests+test-bkup | "/root/files/d.txt" -> "4bd6315d6d7824c4e376847ca7d116738ad2f29a"; /root/bkup/data/4b/d6/31/4bd6315d6d7824c4e376847ca7d116738ad2f29a backing up
    ./tests+test-bkup | "/root/files/sub/dir/e.txt" -> "d929c82d2ee727ccbea9c50c669a71075249899f"; /root/bkup/data/d9/29/c8/d929c82d2ee727ccbea9c50c669a71075249899f backing up
    ./tests+test-bkup | done
    ./tests+test-bkup | writing to /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c
    ./tests+test-bkup | creating backup /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c -> /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c.bkup
    ./tests+test-bkup | writing to /root/bkup/refs
    ./tests+test-bkup | --> RUN test "$(cat /root/bkup/refs)" = "bee07a7f6a5e8ae619273e1a143562cbb5468d7c"
    ./tests+test-bkup | --> RUN ls /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c
    ./tests+test-bkup | /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c
    ./tests+test-bkup | --> RUN ls /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c.bkup
    ./tests+test-bkup | /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c.bkup
    ./tests+test-bkup | --> RUN test "$(cat /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c | sha1sum - | awk '{print $1}')" = "bee07a7f6a5e8ae619273e1a143562cbb5468d7c"
    ./tests+test-bkup | --> RUN acbup --config=acbup.conf
    ./tests+test-bkup | "/root/files/a.txt" -> "d046cd9b7ffb7661e449683313d41f6fc33e3130"; /root/bkup/data/d0/46/cd/d046cd9b7ffb7661e449683313d41f6fc33e3130 already backedup (and verified)
    ./tests+test-bkup | "/root/files/b.txt" -> "bb596efe9e3023a502013767a0559a94a5eea4bc"; /root/bkup/data/bb/59/6e/bb596efe9e3023a502013767a0559a94a5eea4bc already backedup (and verified)
    ./tests+test-bkup | "/root/files/c.txt" -> "d6ed21679f692a68a2202cb9a2ff1e861f97fc63"; /root/bkup/data/d6/ed/21/d6ed21679f692a68a2202cb9a2ff1e861f97fc63 already backedup (and verified)
    ./tests+test-bkup | "/root/files/d.txt" -> "4bd6315d6d7824c4e376847ca7d116738ad2f29a"; /root/bkup/data/4b/d6/31/4bd6315d6d7824c4e376847ca7d116738ad2f29a already backedup (and verified)
    ./tests+test-bkup | "/root/files/sub/dir/e.txt" -> "d929c82d2ee727ccbea9c50c669a71075249899f"; /root/bkup/data/d9/29/c8/d929c82d2ee727ccbea9c50c669a71075249899f already backedup (and verified)
    ./tests+test-bkup | done
    ./tests+test-bkup | writing to /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c
    ./tests+test-bkup | creating backup /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c -> /root/bkup/data/be/e0/7a/bee07a7f6a5e8ae619273e1a143562cbb5468d7c.bkup
    ./tests+test-bkup | writing to /root/bkup/refs
