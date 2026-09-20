package Bunshin::App;
use strict;
use warnings;
use FindBin;
use lib "$FindBin::Bin/..";
use Bunshin::ContentRunner;
use Bunshin::HTTP;
use HTML::Entities ();
use Module::Refresh;
use Quiz;

my $HTML_SHELL = <<~'HTML';
    <!doctype html>
    <html lang="en">
      <head>
        <meta charset="utf-8">
        <title>大吉祥寺.pm</title>
        <style>
          body { font-family: system-ui, sans-serif; margin: 2rem; line-height: 1.6; }
          pre { background: #fee; padding: 1rem; border-radius: 4px; overflow: auto; }
        </style>
      </head>
      <body>
        %s
      </body>
    </html>
    HTML

my $CONTENT_FN = sub { Quiz::page() };
our $RUN_CONTENT_FN = sub {
    my @load_errors;
    {
        local $SIG{__WARN__} = sub {
            my ($msg) = @_;
            if ($msg =~ /Compilation failed in require|aborted due to compilation errors/) {
                push @load_errors, $msg;
                return;
            }
            warn $msg;
        };
        eval { Module::Refresh->refresh; 1 } or push @load_errors, $@;
    }
    die "DaiKichijoji.pm load failed: " . join("\n", @load_errors) if @load_errors;
    return Bunshin::ContentRunner::run(
        content_fn => $CONTENT_FN,
        timeout_ms => 3000,
    );
};

sub init {
    Module::Refresh->refresh;
}

sub handle_conn {
    my ($conn) = @_;

    my $drained = eval { Bunshin::HTTP::drain_request($conn); 1 };
    if (!$drained) {
        Bunshin::HTTP::respond($conn, 400, 'text/plain; charset=utf-8', "bad request: $@");
        return;
    }

    my $result;
    my $ok = eval { $result = $RUN_CONTENT_FN->(); 1 };
    if (!$ok) {
        respond_error($conn, $@);
        return;
    }
    if (!defined $result) {
        respond_error($conn, "content runner returned undef");
        return;
    }
    if (ref $result ne 'HASH') {
        my $type = ref($result) || 'non-ref scalar';
        respond_error($conn, "content runner returned invalid result: $type");
        return;
    }

    for ($result->{status}) {
        if ($_ eq 'ok') {
            Bunshin::HTTP::respond($conn, 200, 'text/html; charset=utf-8', sprintf($HTML_SHELL, $result->{body}));
        } elsif ($_ eq 'died') {
            respond_error($conn, "quiz page died: $result->{error}");
        } elsif ($_ eq 'exited') {
            respond_error($conn, "quiz page exited with code $result->{code}");
        } elsif ($_ eq 'timed_out') {
            respond_error($conn, "quiz page timed out: exceeded $result->{ms}ms");
        } else {
            respond_error($conn, "content runner returned unknown status: $_");
        }
    }
}

sub respond_error {
    my ($conn, $msg) = @_;
    Bunshin::HTTP::respond($conn, 500, 'text/html; charset=utf-8', build_error_page($msg));
}

sub build_error_page {
    my ($msg) = @_;
    my $escaped = HTML::Entities::encode_entities($msg);
    return sprintf($HTML_SHELL, "<h1>Error</h1>\n<pre>$escaped</pre>");
}

1;
