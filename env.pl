#!/usr/bin/env perl

my $vars=`flyctl secrets list | grep -v DIGEST  | awk '{ print \$1 }'`;
my $output=`flyctl ssh console -C 'env' 2>/dev/null`;

my %lookup = map { $_ => 1 } split /\n/, $vars;


for (split /\n/, $output) {
  my ($varname) = split /=/;

  if ($lookup{$varname}) {
    my $var = $_;
    $var =~ s/\r//;

    if ($varname eq "DATABASE_URL") {
      $var =~ s/{{ .ProjectName }}db.flycast/localhost/;
    }
    print "$var\n";
  }
}
