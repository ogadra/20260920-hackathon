package DaiKichijoji;
use strict;

sub content {
    return qr{(\S+)\n\1}s;
}

1;
