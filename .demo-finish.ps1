$ErrorActionPreference='Stop'
$b='http://localhost:5180/api/v1';$h=@{'X-Demo-Role'='dispatcher';'X-Demo-Run-Id'='e30eb2249b6134943d4d7c1f'}
function Get-Api($p){Invoke-RestMethod "$b$p" -Headers $h}
function Post-Api($p,$body){$h['Idempotency-Key']=[guid]::NewGuid().ToString('N');Invoke-RestMethod "$b$p" -Method Post -Headers $h -ContentType application/json -Body ($body|ConvertTo-Json -Depth 10)}
function Command($p,$body){$op=Post-Api $p $body;for($i=0;$i -lt 200;$i++){$s=Get-Api "/operations/$($op.id)";if($s.status -eq 'failed'){throw ($s|ConvertTo-Json -Depth 8)};if($s.status -eq 'succeeded'){return $s};Start-Sleep -Milliseconds 100};throw 'timeout'}
$ids=@('859671df5c2aa9b9949ea787','c8aad6c742a1ed72ce7a83be','fccc3003818a1b3ef14ce36c')
for($step=0;$step -lt 35;$step++){
 $done=$true;$times=@()
 foreach($id in $ids){$o=Get-Api "/orders/$id";if($o.status -eq 'issued'){continue};$done=$false;$body=@{expectedVersion=$o.version}
 if($o.status -eq 'ready'){$body.type='issued';$null=Command "/orders/$id/events" $body;continue}
 $route=Get-Api "/orders/$id/route?revision=$($o.routeRevision)"
 foreach($sh in $o.shipments){$plan=$route.shipments|Where-Object id -eq $sh.id;$body=@{expectedVersion=$o.version;shipmentId=$sh.id}
 if($sh.status -eq 'reserved'){if($sh.fromNodeId -ne '1920000001'){$body.type='packed';$null=Command "/orders/$id/events" $body;break}}
 $seg=$plan.segments|Where-Object {$_.id -eq $sh.nextSegmentId -or $_.id -eq $sh.activeSegmentId}
 if($seg){$clock=[datetimeoffset](Get-Api '/demo/context').clock;$at=if($sh.status -eq 'in_transit'){$seg.scheduledEndAt}else{$seg.scheduledStartAt};$times+=[datetimeoffset]$at
 if($seg.mode -eq 'physical' -and [datetimeoffset]$at -le $clock){$body.segmentId=$seg.id;$body.type=if($sh.status -eq 'in_transit'){'arrived'}else{'handed_over'};$null=Command "/orders/$id/events" $body;break}}
 }
 }
 if($done){break};$clock=[datetimeoffset](Get-Api '/demo/context').clock;$future=$times|Where-Object {$_ -gt $clock}|Sort-Object|Select-Object -First 1
 if($future){$null=Command '/demo/clock/advances' @{to=$future.ToString('o')}}else{$null=Command '/demo/clock/advances' @{to=$clock.AddSeconds(1).ToString('o')}}
}
foreach($id in $ids){Get-Api "/orders/$id"|Select-Object id,mode,status,settlementStatus}


