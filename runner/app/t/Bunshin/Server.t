use strict;
use warnings;
use Test::More;
use FindBin;
use lib "$FindBin::Bin/../..";
use Bunshin::Server;
use IO::Socket::INET;
use POSIX ();

subtest 'run dies without handler' => sub {
    my $ok = eval {
        Bunshin::Server::run(listen_addr => '127.0.0.1', listen_port => 0);
        1;
    };
    ok !$ok, 'died';
    like $@, qr{handler required};
};

subtest 'run dies without listen_addr' => sub {
    my $ok = eval {
        Bunshin::Server::run(listen_port => 0, handler => sub { });
        1;
    };
    ok !$ok, 'died';
    like $@, qr{listen_addr required};
};

subtest 'run dies without listen_port' => sub {
    my $ok = eval {
        Bunshin::Server::run(listen_addr => '127.0.0.1', handler => sub { });
        1;
    };
    ok !$ok, 'died';
    like $@, qr{listen_port required};
};

subtest 'run binds, forks, dispatches to handler with SIG{CHLD} = IGNORE' => sub {
    my $probe = IO::Socket::INET->new(
        LocalAddr => '127.0.0.1',
        LocalPort => 0,
        Proto     => 'tcp',
        Listen    => 1,
        ReuseAddr => 1,
    ) or die "probe: $!";
    my $port = $probe->sockport;
    $probe->close;

    pipe(my $child_reads, my $parent_writes) or die "pipe: $!";

    my $server_pid = fork;
    die "fork: $!" if !defined $server_pid;
    if ($server_pid == 0) {
        close $child_reads;
        $parent_writes->autoflush(1);
        Bunshin::Server::run(
            listen_addr => '127.0.0.1',
            listen_port => $port,
            handler     => sub {
                my ($conn) = @_;
                syswrite $parent_writes, "chld=$SIG{CHLD}\n";
                syswrite $conn, "OK";
            },
        );
        POSIX::_exit(0);
    }
    close $parent_writes;

    my $client;
    for (1 .. 30) {
        $client = IO::Socket::INET->new(PeerAddr => "127.0.0.1:$port");
        last if $client;
        select undef, undef, undef, 0.1;
    }
    ok $client, 'connected to server';

    my $reply = do { local $/; <$client> };
    close $client;

    my $signal_line = <$child_reads>;
    close $child_reads;
    chomp $signal_line if defined $signal_line;

    kill 'TERM', $server_pid;
    waitpid $server_pid, 0;

    is $reply,       'OK',          'handler wrote through the connection';
    is $signal_line, 'chld=IGNORE', '$SIG{CHLD} is IGNORE in the fork child';
};

subtest 'run keeps the loop alive when handler dies' => sub {
    my $probe = IO::Socket::INET->new(
        LocalAddr => '127.0.0.1', LocalPort => 0, Proto => 'tcp',
        Listen => 1, ReuseAddr => 1,
    ) or die "probe: $!";
    my $port = $probe->sockport;
    $probe->close;

    my $server_pid = fork;
    die "fork: $!" if !defined $server_pid;
    if ($server_pid == 0) {
        Bunshin::Server::run(
            listen_addr => '127.0.0.1',
            listen_port => $port,
            handler     => sub { die "handler exploded\n" },
        );
        POSIX::_exit(0);
    }

    my $c1;
    for (1 .. 30) {
        $c1 = IO::Socket::INET->new(PeerAddr => "127.0.0.1:$port");
        last if $c1;
        select undef, undef, undef, 0.1;
    }
    ok $c1, 'first connect';
    close $c1 if $c1;

    my $c2 = IO::Socket::INET->new(PeerAddr => "127.0.0.1:$port");
    ok $c2, 'second connect after handler died';
    close $c2 if $c2;

    kill 'TERM', $server_pid;
    waitpid $server_pid, 0;
};

done_testing;
