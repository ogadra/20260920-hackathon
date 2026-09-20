package Quiz;
use strict;
use warnings;
use utf8;
use Encode ();
use HTML::Entities ();
use re qw(regexp_pattern);

our $MAP = join "\n",
    '吉祥寺久我山永福町明大前下北沢渋谷',
    '渋谷恵比寿大崎大井町',
    '大井町品川高輪ゲートウェイ田町浜松町新橋有楽町東京',
    '東京神田御茶ノ水四ツ谷新宿中野荻窪吉祥寺';

our $ANSWER = { '吉祥寺' => 1, '大井町' => 1 };

my $UNSAFE_HTML = '<>&"';

sub _esc { HTML::Entities::encode_entities($_[0], $UNSAFE_HTML) }

sub evaluate {
    my (%opts) = @_;
    my $re  = $opts{re} // die "re required\n";
    my $map = $MAP;

    my @matches;
    while ($map =~ /$re/g) {
        my $whole = substr($map, $-[0], $+[0] - $-[0]);
        my $pick  = defined $1 ? $1 : $whole;
        push @matches, {
            pick  => $pick,
            whole => $whole,
            start => $-[0],
            end   => $+[0],
        };
        pos($map) = $+[0] + 1 if $+[0] == $-[0];
    }
    return \@matches;
}

sub judge {
    my (%opts) = @_;
    my $matches = $opts{matches} // die "matches required\n";
    my %count;
    $count{ $_->{pick} }++ for @$matches;
    my $expected = join '|', sort keys %$ANSWER;
    my $actual   = join '|', sort keys %count;

    return { status => 'wrong', message => '不正解…' }
        if $actual ne $expected;
    return { status => 'wrong', message => '不正解… 駅は合っていますが、同じ駅が2回マッチしています' }
        if grep { $_ > 1 } values %count;
    return { status => 'correct', message => '正解！' };
}

sub regex_display {
    my ($re) = @_;
    my ($pat, $mods) = regexp_pattern($re);
    $mods =~ tr/u//d;
    my $src = length $mods ? "$pat/$mods" : $pat;
    my $bytes = Encode::encode('UTF-8', $src);
    return { pattern => $pat, mods => $mods, source => $src, bytes => length $bytes };
}

sub highlight_map {
    my (%opts) = @_;
    my $matches = $opts{matches} // die "matches required\n";
    my $map     = $MAP;
    my @sorted  = sort { $a->{start} <=> $b->{start} } @$matches;

    my ($out, $cursor) = ('', 0);
    for my $m (@sorted) {
        next if $m->{start} < $cursor;
        next if $m->{end} == $m->{start};
        $out .= _esc(substr($map, $cursor, $m->{start} - $cursor));
        $out .= '<mark>' . _esc($m->{whole}) . '</mark>';
        $cursor = $m->{end};
    }
    $out .= _esc(substr($map, $cursor));
    return $out;
}

sub page {
    my $content_sub = DaiKichijoji->can('content')
        or die "DaiKichijoji::content is not defined\n";
    my $re = $content_sub->();
    die "DaiKichijoji::content must return a compiled regex (qr//)\n"
        unless ref $re eq 'Regexp';

    my $rd      = regex_display($re);
    my $matches = evaluate(re => $re);
    my $judged  = judge(matches => $matches);

    my $map_html = highlight_map(matches => $matches);
    my $re_html  = _esc($rd->{source});
    my $msg_html = _esc($judged->{message});

    return <<~"HTML";
        <h1>Perl正規表現クイズ!</h1>
        <section class="quiz">
          <h2>問題</h2>
          <p>
            Magnolia.Kさんは最寄り駅から電車に乗って、いくつかの路線を乗り継ぎ、最寄り駅まで戻ってきました。<br/>
            下の各行は、乗った路線ごとの停車駅を順に並べたものです。
          </p>
          <pre class="map">$map_html</pre>
          <p>
            2回登場する駅のうち、漢字3文字の駅はどれですか？<br/>
            正規表現で重複なく抜き出して解答してください。
          </p>
          <p>正規表現: <code>$re_html</code> ($rd->{bytes} bytes)</p>
          <p>
            マッチした駅: <code>@{[ join ', ', map { _esc($_->{pick}) } @$matches ]}</code></p>
          <p class="verdict verdict-$judged->{status}">$msg_html</p>
        </section>
        HTML
}

1;
