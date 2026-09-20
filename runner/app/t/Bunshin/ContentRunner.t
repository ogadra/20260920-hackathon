use strict;
use warnings;
use Test::More;
use FindBin;
use lib "$FindBin::Bin/../..";
use Bunshin::ContentRunner;
use POSIX ();
use Time::HiRes ();

$SIG{ALRM} = sub { die "test watchdog: hang detected\n" };
alarm 30;

subtest 'ok: content is passed through as bytes' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { "hello" });
    is $r->{status}, 'ok';
    is $r->{body},   'hello';
};

subtest 'ok: utf8-flagged content is encoded before pipe transfer' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub {
        my $s = "\x{3042}\x{3044}";
        utf8::upgrade($s);
        return $s;
    });
    is $r->{status}, 'ok';
    is $r->{body}, "\xe3\x81\x82\xe3\x81\x84", 'utf8 bytes on the wire';
};

subtest 'ok: payload larger than the 64KB pipe buffer round-trips intact' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { 'x' x (128 * 1024) });
    is $r->{status}, 'ok';
    is length $r->{body}, 128 * 1024;
};

subtest 'died: die captured with longmess stack trace' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { die "boom\n" });
    is $r->{status}, 'died';
    like $r->{error}, qr{boom};
    like $r->{error}, qr{Bunshin::ContentRunner}, 'stack trace present';
};

subtest 'died: content returning undef reported as died' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { undef });
    is $r->{status}, 'died';
    like $r->{error}, qr{returned undef};
};

subtest 'exited: exit(42) captured as exit code' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { exit 42 });
    is $r->{status}, 'exited';
    is $r->{code},   42;
};

subtest 'exited: exit(0) distinguished from ok because tag is missing' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { exit 0 });
    is $r->{status}, 'exited';
    is $r->{code},   0;
};

subtest 'exited: exit(1) without a written tag falls through to exited' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { exit 1 });
    is $r->{status}, 'exited';
    is $r->{code},   1;
};

subtest 'exited: POSIX::_exit(7) also captured' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub { POSIX::_exit(7) });
    is $r->{status}, 'exited';
    is $r->{code},   7;
};

subtest 'died: uncatchable signal reported as killed by signal' => sub {
    my $r = Bunshin::ContentRunner::run(content_fn => sub {
        kill 'KILL', $$;
        sleep 5;
        "unreachable";
    });
    is $r->{status}, 'died';
    like $r->{error}, qr{killed by signal 9};
};

subtest 'ok: parent SIG{CHLD} = IGNORE does not break waitpid in the runner' => sub {
    local $SIG{CHLD} = 'IGNORE';
    my $r = Bunshin::ContentRunner::run(content_fn => sub { "post-ignore" });
    is $r->{status}, 'ok';
    is $r->{body},   'post-ignore';
};

subtest 'timed_out: content exceeds timeout budget' => sub {
    my $t0 = Time::HiRes::time();
    my $r  = Bunshin::ContentRunner::run(
        content_fn => sub { sleep 5; "unreachable" },
        timeout_ms => 200,
    );
    my $elapsed_ms = (Time::HiRes::time() - $t0) * 1000;
    is $r->{status}, 'timed_out';
    is $r->{ms},     200;
    cmp_ok $elapsed_ms, '<', 2000, 'timed out before content sleep completed';
};

subtest 'timed_out: fires when parent has SIG{ALRM} = IGNORE' => sub {
    local $SIG{ALRM} = 'IGNORE';
    my $r = Bunshin::ContentRunner::run(
        content_fn => sub { sleep 5; "unreachable" },
        timeout_ms => 200,
    );
    is $r->{status}, 'timed_out', 'timeout fires when parent ignores SIGALRM';
};

subtest 'timed_out: fires when content sets SIG{ALRM} = IGNORE' => sub {
    my $r = Bunshin::ContentRunner::run(
        content_fn => sub { $SIG{ALRM} = 'IGNORE'; sleep 5; "unreachable" },
        timeout_ms => 200,
    );
    is $r->{status}, 'timed_out', 'timeout fires when content ignores SIGALRM';
};

subtest 'ok: fast content finishes before the timeout budget' => sub {
    my $r = Bunshin::ContentRunner::run(
        content_fn => sub { "quick" },
        timeout_ms => 3000,
    );
    is $r->{status}, 'ok';
    is $r->{body},   'quick';
};

subtest 'ok: large payload with timeout drives multiple sysread iterations' => sub {
    my $r = Bunshin::ContentRunner::run(
        content_fn => sub { 'x' x (128 * 1024) },
        timeout_ms => 3000,
    );
    is $r->{status}, 'ok';
    is length $r->{body}, 128 * 1024;
};

subtest 'exited: exit(42) classifies as exited when timeout_ms is set' => sub {
    my $r = Bunshin::ContentRunner::run(
        content_fn => sub { exit 42 },
        timeout_ms => 3000,
    );
    is $r->{status}, 'exited';
    is $r->{code},   42;
};

subtest 'security: grandchild does not inherit unrelated parent fds' => sub {
    pipe(my $probe_reader, my $probe_writer) or die "pipe: $!";
    my $probe_fd = fileno($probe_writer);
    my $r = Bunshin::ContentRunner::run(content_fn => sub {
        return open(my $dup, ">&=", $probe_fd) ? "leaked fd $probe_fd" : "closed";
    });
    close $probe_reader;
    close $probe_writer;
    is $r->{status}, 'ok';
    is $r->{body}, 'closed', "parent's probe fd is not visible from the grandchild";
};

subtest 'run: croaks when content_fn is missing (programmer error, not a runtime died)' => sub {
    eval { Bunshin::ContentRunner::run() };
    like $@, qr{content_fn required};
};

alarm 0;
done_testing;
