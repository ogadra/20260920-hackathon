use strict;
use warnings;
use utf8;
use Test::More;
use FindBin;
use lib "$FindBin::Bin/..";
use Quiz;

binmode Test::More->builder->$_, ':encoding(UTF-8)'
    for qw(output failure_output todo_output);

sub with_content {
    my ($answer, $fn) = @_;
    no warnings 'redefine', 'once';
    local *DaiKichijoji::content = sub { $answer };
    $fn->();
}

subtest 'MAP: 3-char windows repeat only for 吉祥寺 and 大井町' => sub {
    my %count;
    my $len = length $Quiz::MAP;
    for my $i (0 .. $len - 3) {
        my $window = substr($Quiz::MAP, $i, 3);
        next if $window =~ /\n/;
        $count{$window}++;
    }
    my @repeated = sort grep { $count{$_} >= 2 } keys %count;
    is_deeply \@repeated, ['吉祥寺', '大井町'],
        'only 吉祥寺 and 大井町 repeat among all 3-char (no-newline) windows';
};

subtest 'evaluate: literal 吉祥寺|大井町 picks up both stations' => sub {
    my $matches = Quiz::evaluate(re => qr{吉祥寺|大井町});
    my %set = map { $_->{pick} => 1 } @$matches;
    is_deeply [sort keys %set], ['吉祥寺', '大井町'], 'match set = expected';
    is scalar(@$matches), 4, 'both stations matched exactly twice';
};

subtest 'evaluate: greedy /s backref consumes the whole string in one match' => sub {
    my $matches = Quiz::evaluate(re => qr{(...).*\1}s);
    my %set = map { $_->{pick} => 1 } @$matches;
    is scalar(@$matches), 1, 'greedy .* leaves nothing for a second iteration';
    is_deeply [sort keys %set], ['吉祥寺'], 'only the outermost pair is picked';
};

subtest 'evaluate: /s lookahead backref captures both stations' => sub {
    my $matches = Quiz::evaluate(re => qr{(...)(?=.*\1)}s);
    my %set = map { $_->{pick} => 1 } @$matches;
    is_deeply [sort keys %set], ['吉祥寺', '大井町'], 'captures fill the answer set';
};

subtest 'evaluate: no /s + backref finds nothing (dot skips newline)' => sub {
    my $matches = Quiz::evaluate(re => qr{(...).*\1});
    is scalar(@$matches), 0, 'no cross-line matches';
};

subtest 'evaluate: (...)\n\1 captures only the adjacent-line pair (大井町)' => sub {
    my $matches = Quiz::evaluate(re => qr{(...)\n\1});
    my %set = map { $_->{pick} => 1 } @$matches;
    is_deeply [sort keys %set], ['大井町'], 'the 3-char neighbour is 大井町';
};

subtest 'evaluate: /^...|...$/ picks up 吉祥寺 only (both string ends)' => sub {
    my $matches = Quiz::evaluate(re => qr{^...|...$});
    my %set = map { $_->{pick} => 1 } @$matches;
    is_deeply [sort keys %set], ['吉祥寺'], 'only 吉祥寺 at the string ends';
};

subtest 'judge: literal 吉祥寺|大井町 double-matches and is wrong' => sub {
    my $m = Quiz::evaluate(re => qr{吉祥寺|大井町});
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
    like $v->{message}, qr{同じ駅が2回マッチ}, 'dedicated duplicate message';
};

subtest 'judge: a wrong set with duplicates gets the plain message' => sub {
    my $m = Quiz::evaluate(re => qr{渋谷});
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
    is $v->{message}, '不正解…', 'set mismatch outranks the duplicate hint';
};

subtest 'judge: /s lookahead backref is correct' => sub {
    my $m = Quiz::evaluate(re => qr{(...)(?=.*\1)}s);
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'correct';
};

subtest 'judge: matching only 吉祥寺 is wrong' => sub {
    my $m = Quiz::evaluate(re => qr{(...).*\1}s);
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
};

subtest 'judge: matching only 大井町 is wrong' => sub {
    my $m = Quiz::evaluate(re => qr{(...)\n\1});
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
};

subtest 'judge: no matches is wrong' => sub {
    my $m = Quiz::evaluate(re => qr{ZZZZ});
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
};

subtest 'judge: unexpected match set is wrong' => sub {
    my $m = Quiz::evaluate(re => qr{渋谷});
    my $v = Quiz::judge(matches => $m);
    is $v->{status}, 'wrong';
};

subtest 'regex_display: literal answer weighs 19 bytes' => sub {
    my $rd = Quiz::regex_display(qr{吉祥寺|大井町});
    is $rd->{bytes}, 19, '3 kanji x 6 + | = 19 bytes';
    is $rd->{mods}, '', 'implicit /u is hidden';
};

subtest 'regex_display: /s backref answer weighs 11 bytes' => sub {
    my $rd = Quiz::regex_display(qr{(...).*\1}s);
    is $rd->{bytes}, 11, 'pattern (9) + /s (2) = 11';
    is $rd->{mods}, 's';
};

subtest 'highlight_map: matched spans are wrapped in <mark>' => sub {
    my $m = Quiz::evaluate(re => qr{吉祥寺|大井町});
    my $html = Quiz::highlight_map(matches => $m);
    like $html, qr{<mark>吉祥寺</mark>};
    like $html, qr{<mark>大井町</mark>};
    unlike $html, qr{<mark></mark>}, 'no empty marks';
};

subtest 'highlight_map: zero-width matches leave no empty marks' => sub {
    my $m = Quiz::evaluate(re => qr{(?=大井町)});
    my $html = Quiz::highlight_map(matches => $m);
    unlike $html, qr{<mark></mark>}, 'no empty marks';
    unlike $html, qr{<mark>}, 'zero-width matches highlight nothing';
};

subtest 'highlight_map: HTML metacharacters in the map are escaped' => sub {
    local $Quiz::MAP = 'abc<xy>&"z';
    my $m = Quiz::evaluate(re => qr{abc});
    my $html = Quiz::highlight_map(matches => $m);
    like $html, qr{&lt;xy&gt;},   '< > escaped';
    like $html, qr{&amp;},        '& escaped';
    like $html, qr{&quot;},       '" escaped';
    unlike $html, qr{<xy>},       'raw tag not present';
};

subtest 'page: HTML metacharacters in the regex source are escaped' => sub {
    with_content(qr{<script>}, sub {
        my $html = Quiz::page();
        like $html, qr{&lt;script&gt;}, 'user-supplied regex is escaped';
        unlike $html, qr{<script>},     'raw regex not embedded';
    });
};

subtest 'evaluate: zero-width lookahead scan terminates and advances' => sub {
    local $Quiz::MAP = 'aXaXa';
    my $matches = Quiz::evaluate(re => qr{(?=a)});
    is scalar(@$matches), 3, 'one zero-width hit per a';
    is_deeply [map { $_->{start} } @$matches], [0, 2, 4], 'positions advance';
};

subtest 'required opts: subs die when a critical arg is missing' => sub {
    my @cases = (
        [sub { Quiz::evaluate() },       qr{re required} ],
        [sub { Quiz::judge() },          qr{matches required} ],
        [sub { Quiz::highlight_map() },  qr{matches required} ],
    );
    for my $c (@cases) {
        my ($fn, $pat) = @$c;
        my $ok = eval { $fn->(); 1 };
        ok !$ok, 'died';
        like $@, $pat, "matches $pat";
    }
};

subtest 'page: renders question and verdict from the content answer' => sub {
    with_content(qr{(...)(?=.*\1)}s, sub {
        my $html = Quiz::page();
        like $html, qr{Magnolia.Kさん}, 'question copy present';
        like $html, qr{verdict-correct}, 'verdict class reflects status';
    });
};

subtest 'page: a wrong answer renders verdict-wrong' => sub {
    with_content(qr{渋谷}, sub {
        my $html = Quiz::page();
        like $html, qr{verdict-wrong}, 'verdict class reflects status';
    });
};

subtest 'page: lists the picked stations next to the regex' => sub {
    with_content(qr{(...)(?=.*\1)}s, sub {
        my $html = Quiz::page();
        like $html, qr{マッチした駅.*<code>吉祥寺, 大井町</code>}s, 'picks are rendered in match order';
    });
};

subtest 'page: dies when DaiKichijoji::content is not defined' => sub {
    ok !DaiKichijoji->can('content'), 'precondition: this test file never loads DaiKichijoji';
    my $ok = eval { Quiz::page(); 1 };
    ok !$ok, 'died';
    like $@, qr{DaiKichijoji::content is not defined};
};

subtest 'page: dies when content returns something other than qr//' => sub {
    with_content('吉祥寺|大井町', sub {
        my $ok = eval { Quiz::page(); 1 };
        ok !$ok, 'died';
        like $@, qr{must return a compiled regex};
    });
};

done_testing;
