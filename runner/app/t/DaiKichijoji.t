use strict;
use warnings;
use utf8;
use Test::More;
use FindBin;
use lib "$FindBin::Bin/..";
use DaiKichijoji;

binmode Test::More->builder->$_, ':encoding(UTF-8)'
    for qw(output failure_output todo_output);

subtest 'content: the initial answer catches line-crossing repeats but misses 吉祥寺' => sub {
    require Quiz;
    my $matches = Quiz::evaluate(re => DaiKichijoji::content());
    my %set = map { $_->{pick} => 1 } @$matches;
    is_deeply [sort keys %set], ['大井町', '東京', '渋谷'],
        '吉祥寺 at the string ends is not adjacent across lines';
    is Quiz::judge(matches => $matches)->{status}, 'wrong',
        'participants still have to narrow the answer down';
};

done_testing;
