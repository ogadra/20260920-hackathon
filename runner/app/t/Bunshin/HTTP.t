use strict;
use warnings;
use Test::More;
use FindBin;
use lib "$FindBin::Bin/../..";
use Bunshin::HTTP;
use Socket qw(AF_UNIX SOCK_STREAM PF_UNSPEC);

# ---------- respond ----------

subtest 'respond writes a well-formed HTTP/1.1 response' => sub {
    my $out = '';
    open my $fh, '>', \$out or die "open scalar: $!";
    Bunshin::HTTP::respond($fh, 200, 'text/plain; charset=utf-8', 'hi');
    close $fh;
    like $out, qr{^HTTP/1\.1 200 OK\r\n}, 'status line with known reason phrase';
    like $out, qr{Content-Type: text/plain; charset=utf-8\r\n}, 'Content-Type header';
    like $out, qr{Content-Length: 2\r\n}, 'Content-Length in bytes';
    like $out, qr{Connection: close\r\n}, 'Connection: close';
    like $out, qr{\r\n\r\nhi\z}, 'body follows the header terminator';
};

subtest 'respond measures Content-Length in bytes, not characters' => sub {
    my $out = '';
    open my $fh, '>', \$out or die;
    my $body = "\x{3042}";
    Bunshin::HTTP::respond($fh, 200, 'text/plain', $body);
    close $fh;
    like $out, qr{Content-Length: 3\r\n}, 'byte count of a utf8-flagged string';
};

subtest 'respond dies on an unknown status code' => sub {
    my $out = '';
    open my $fh, '>', \$out or die;
    my $ok = eval { Bunshin::HTTP::respond($fh, 999, 'text/plain', ''); 1 };
    close $fh;
    ok !$ok, 'respond died';
    like $@, qr{unknown status 999}, 'error names the offending status';
};

subtest 'respond dies on undef status / ctype / body' => sub {
    my $out = '';
    open my $fh, '>', \$out or die;
    my $ok;
    $ok = eval { Bunshin::HTTP::respond($fh, undef, 'text/plain', ''); 1 };
    ok !$ok, 'died on undef status';
    like $@, qr{status required};
    $ok = eval { Bunshin::HTTP::respond($fh, 200, undef, ''); 1 };
    ok !$ok, 'died on undef content type';
    like $@, qr{content type required};
    $ok = eval { Bunshin::HTTP::respond($fh, 200, 'text/plain', undef); 1 };
    ok !$ok, 'died on undef body';
    like $@, qr{body required};
    close $fh;
};

# ---------- drain_request ----------

sub make_reader {
    my ($data) = @_;
    my ($server, $client);
    socketpair($server, $client, AF_UNIX, SOCK_STREAM, PF_UNSPEC)
        or die "socketpair: $!";
    syswrite($client, $data) if length $data;
    shutdown($client, 1) or die "shutdown: $!";
    return ($server, $client);
}

subtest 'drain_request consumes a bare GET' => sub {
    my ($s, $c) = make_reader("GET /a/b HTTP/1.1\r\nHost: x\r\n\r\n");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok $ok, 'no die';
    is $@, '', '$@ cleared';
};

subtest 'drain_request consumes a PUT body via Content-Length' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok $ok, 'no die';
};

subtest 'drain_request accepts LF-only line endings' => sub {
    my ($s, $c) = make_reader("GET /lf HTTP/1.1\n\n");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok $ok, 'no die';
};

subtest 'drain_request treats missing Content-Length as 0' => sub {
    my ($s, $c) = make_reader("POST /x HTTP/1.1\r\nHost: y\r\n\r\n");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok $ok, 'no die';
};

subtest 'drain_request dies on EOF before the header terminator' => sub {
    my ($s, $c) = make_reader("GET /");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok !$ok, 'died';
    like $@, qr{unexpected EOF};
};

subtest 'drain_request dies when the body is shorter than Content-Length' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\nContent-Length: 100\r\n\r\nshort");
    my $ok = eval { Bunshin::HTTP::drain_request($s); 1 };
    close $s;
    close $c;
    ok !$ok, 'died';
    like $@, qr{body truncated};
};

subtest 'drain_request consumes exactly Content-Length bytes' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\nContent-Length: 5\r\n\r\nhelloEXTRA");
    Bunshin::HTTP::drain_request($s);
    my $rest = '';
    sysread $s, $rest, 100;
    close $s;
    close $c;
    is $rest, 'EXTRA', 'trailing bytes past Content-Length remain in the socket';
};

subtest 'drain_request accepts lowercase content-length header' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\ncontent-length: 5\r\n\r\nhelloAFTER");
    Bunshin::HTTP::drain_request($s);
    my $rest = '';
    sysread $s, $rest, 100;
    close $s;
    close $c;
    is $rest, 'AFTER', 'body consumed via case-insensitive header';
};

subtest 'drain_request treats non-numeric Content-Length as 0' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\nContent-Length: abc\r\n\r\nBODY");
    Bunshin::HTTP::drain_request($s);
    my $rest = '';
    sysread $s, $rest, 100;
    close $s;
    close $c;
    is $rest, 'BODY', 'body left untouched when Content-Length is non-numeric';
};

subtest 'drain_request handles Content-Length: 0 without reading body' => sub {
    my ($s, $c) = make_reader("PUT / HTTP/1.1\r\nContent-Length: 0\r\n\r\nAFTER0");
    Bunshin::HTTP::drain_request($s);
    my $rest = '';
    sysread $s, $rest, 100;
    close $s;
    close $c;
    is $rest, 'AFTER0', 'no bytes consumed when Content-Length is 0';
};

done_testing;
