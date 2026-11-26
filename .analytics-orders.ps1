$ErrorActionPreference='Stop'
$b='http://localhost:5180/api/v1';$h=@{'X-Demo-Role'='dispatcher';'X-Demo-Run-Id'='f594e00dba8e52a0b251b677'}
function Get-Api($p){Invoke-RestMethod "$b$p" -Headers $h}
function Post-Api($p,$body){$h['Idempotency-Key']=[guid]::NewGuid().ToString('N');Invoke-RestMethod "$b$p" -Method Post -Headers $h -ContentType application/json -Body ($body|ConvertTo-Json -Depth 10)}
function Command($p,$body){$op=Post-Api $p $body;for($i=0;$i -lt 200;$i++){$s=Get-Api "/operations/$($op.id)";if($s.status -eq 'failed'){throw ($s|ConvertTo-Json -Depth 8)};if($s.status -eq 'succeeded'){return $s};Start-Sleep -Milliseconds 100};throw 'timeout'}
$ids=@()
foreach($product in @('2100000001','2100000002','2100000003')){foreach($policy in @('mesh_only','hub_only')){$q=Post-Api '/quotes' @{pickupPointId='2500000003';items=@(@{productId=$product;quantity=1})};$opt=$q.options|Where-Object {$_.policy -eq $policy -and $_.feasible};if(!$opt){throw "No $policy for $product"};$op=Command '/orders' @{quoteId=$q.id;optionId=$opt.id};$ids+=$op.resourceId;Write-Output "CREATED $policy $product $($op.resourceId)"}}
for($step=0;$step -lt 35;$step++){
 $done=$true;$times=@()
 foreach($id in $ids){$o=Get-Api "/orders/$id";if($o.status -eq 'issued'){continue};$done=$false;$body=@{expectedVersion=$o.version}
 if($o.status -eq 'ready'){$body.type='issued';$null=Command "/orders/$id/events" $body;continue}
 $route=Get-Api "/orders/$id/route?revision=$($o.routeRevision)"
 foreach($sh in $o.shipments){$plan=$route.shipments|Where-Object id -eq $sh.id;$body=@{expectedVersion=$o.version;shipmentId=$sh.id}
 if($sh.status -eq 'reserved'){$node=Get-Api "/nodes/$($sh.fromNodeId)";if($node.mode -ne 'simulated'){$body.type='packed';$null=Command "/orders/$id/events" $body;break}}
 $seg=$plan.segments|Where-Object {$_.id -eq $sh.nextSegmentId -or $_.id -eq $sh.activeSegmentId}
 if($seg){$clock=[datetimeoffset](Get-Api '/demo/context').clock;$at=if($sh.status -eq 'in_transit'){$seg.scheduledEndAt}else{$seg.scheduledStartAt};$times+=[datetimeoffset]$at
 if($seg.mode -eq 'physical' -and [datetimeoffset]$at -le $clock){$body.segmentId=$seg.id;$body.type=if($sh.status -eq 'in_transit'){'arrived'}else{'handed_over'};$null=Command "/orders/$id/events" $body;break}}
 }
 }
 if($done){break};$clock=[datetimeoffset](Get-Api '/demo/context').clock;$future=$times|Where-Object {$_ -gt $clock}|Sort-Object|Select-Object -First 1
 if($future){$null=Command '/demo/clock/advances' @{to=$future.ToString('o')}}else{$null=Command '/demo/clock/advances' @{to=$clock.AddSeconds(1).ToString('o')}}
}
foreach($id in $ids){Get-Api "/orders/$id"|Select-Object id,mode,status,settlementStatus}
