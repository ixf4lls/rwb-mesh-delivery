$ErrorActionPreference='Stop'
$b='http://localhost:5180/api/v1';$h=@{'X-Demo-Role'='dispatcher';'X-Demo-Run-Id'='e30eb2249b6134943d4d7c1f'}
function Get-Api($p){Invoke-RestMethod "$b$p" -Headers $h}
function Post-Api($p,$body){$h['Idempotency-Key']=[guid]::NewGuid().ToString('N');Invoke-RestMethod "$b$p" -Method Post -Headers $h -ContentType application/json -Body ($body|ConvertTo-Json -Depth 10)}
function Command($p,$body){$op=Post-Api $p $body;for($i=0;$i -lt 200;$i++){$s=Get-Api "/operations/$($op.id)";if($s.status -eq 'failed'){throw ($s|ConvertTo-Json -Depth 8)};if($s.status -eq 'succeeded'){return $s};Start-Sleep -Milliseconds 100};throw 'timeout'}
foreach($product in @('1100000001','1100000005')){
$q=Post-Api '/quotes' @{pickupPointId='1500000001';items=@(@{productId=$product;quantity=1})};$opt=$q.options|Where-Object {$_.policy -eq 'mesh_only' -and $_.feasible};$op=Command '/orders' @{quoteId=$q.id;optionId=$opt.id};$id=$op.resourceId;$o=Get-Api "/orders/$id";$null=Command "/orders/$id/events" @{type='packed';shipmentId=$o.shipments[0].id;expectedVersion=$o.version};$o=Get-Api "/orders/$id";$null=Command "/orders/$id/events" @{type='issued';expectedVersion=$o.version};Write-Output "S1 ISSUED $id"
}
foreach($p in @('/sellers/1300000001/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z','/pickup-points/1500000001/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z')){Get-Api $p|ConvertTo-Json -Depth 5}
