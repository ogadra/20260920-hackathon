#!/usr/bin/perl
use strict;
use warnings;
use FindBin;
use lib $FindBin::Bin;
use Bunshin::Server;
use Bunshin::App;
use DaiKichijoji;

Bunshin::App::init();
Bunshin::Server::run(
    listen_addr => '0.0.0.0',
    listen_port => 5000,
    handler     => \&Bunshin::App::handle_conn,
);
