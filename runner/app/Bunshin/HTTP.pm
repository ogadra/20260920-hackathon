package Bunshin::HTTP;
use strict;
use warnings;

my %REASON_PHRASES = (
    200 => 'OK',
    201 => 'Created',
    202 => 'Accepted',
    204 => 'No Content',
    301 => 'Moved Permanently',
    302 => 'Found',
    303 => 'See Other',
    304 => 'Not Modified',
    307 => 'Temporary Redirect',
    308 => 'Permanent Redirect',
    400 => 'Bad Request',
    401 => 'Unauthorized',
    403 => 'Forbidden',
    404 => 'Not Found',
    405 => 'Method Not Allowed',
    409 => 'Conflict',
    500 => 'Internal Server Error',
    502 => 'Bad Gateway',
    503 => 'Service Unavailable',
    504 => 'Gateway Timeout',
);

sub drain_request {
    my ($conn) = @_;

    my $header = '';
    while ($header !~ /\r?\n\r?\n\z/) {
        my $byte;
        my $n = sysread $conn, $byte, 1;
        die "read: $!\n" if !defined $n;
        die "unexpected EOF before end of headers\n" if $n == 0;
        $header .= $byte;
    }

    my $content_length = 0;
    if ($header =~ /^Content-Length:\s*(\d+)/im) {
        $content_length = $1 + 0;
    }

    my $drained = 0;
    while ($drained < $content_length) {
        my $chunk;
        my $n = sysread $conn, $chunk, $content_length - $drained;
        die "read body: $!\n" if !defined $n;
        die "body truncated: expected $content_length bytes, got $drained\n" if $n == 0;
        $drained += $n;
    }
    return;
}

sub respond {
    my ($conn, $status, $ctype, $body) = @_;
    die "respond: status required\n"       unless defined $status;
    die "respond: content type required\n" unless defined $ctype;
    die "respond: body required\n"         unless defined $body;
    my $reason = $REASON_PHRASES{$status}
        // die "respond: unknown status $status\n";
    utf8::encode($body) if utf8::is_utf8($body);
    my $len = length $body;
    print $conn "HTTP/1.1 $status $reason\r\n";
    print $conn "Content-Type: $ctype\r\n";
    print $conn "Content-Length: $len\r\n";
    print $conn "Connection: close\r\n";
    print $conn "\r\n";
    print $conn $body;
}

1;
