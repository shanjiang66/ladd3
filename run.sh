./cloudflared tunnel --url http://127.0.0.1:10000 --no-autoupdate --edge-ip-version 4 --protocol http2 >argo.log 2>&1 &
./lade/lade run &
