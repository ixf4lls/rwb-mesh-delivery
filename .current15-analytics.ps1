$ErrorActionPreference='Stop'
$b='http://127.0.0.1:5180/api/v1';$h=@{'X-Demo-Role'='dispatcher';'X-Demo-Run-Id'='78e5522356c9df01d73e5f42'}
function Get-Api($p){Invoke-RestMethod "$b$p" -Headers $h}
function Post-Api($p,$body){$h['Idempotency-Key']=[guid]::NewGuid().ToString('N');Invoke-RestMethod "$b$p" -Method Post -Headers $h -ContentType application/json -Body ($body|ConvertTo-Json -Depth 10)}
function Command($p,$body){$op=Post-Api $p $body;for($i=0;$i -lt 200;$i++){$s=Get-Api "/operations/$($op.id)";if($s.status -eq 'failed'){throw ($s|ConvertTo-Json -Depth 8)};if($s.status -eq 'succeeded'){return $s};Start-Sleep -Milliseconds 100};throw 'timeout'}
foreach($product in @('1100000001','1100000002','1100000003')){for($n=0;$n -lt 2;$n++){
$point=if($product -eq '1100000002'){'1500000002'}else{'1500000001'}
$q=Post-Api '/quotes' @{pickupPointId=$point;items=@(@{productId=$product;quantity=1})};$opt=$q.options|Where-Object {$_.policy -eq 'mesh_only' -and $_.feasible};if(!$opt){throw 'No feasible option'};$op=Command '/orders' @{quoteId=$q.id;optionId=$opt.id};$id=$op.resourceId;$o=Get-Api "/orders/$id";$null=Command "/orders/$id/events" @{type='packed';shipmentId=$o.shipments[0].id;expectedVersion=$o.version};$o=Get-Api "/orders/$id";$null=Command "/orders/$id/events" @{type='issued';expectedVersion=$o.version};Write-Output "ISSUED $product $point $id"
}}
foreach($seller in @('1300000001','1300000002','1300000003')){Get-Api "/sellers/$seller/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z"|ConvertTo-Json -Depth 5}
foreach($point in @('1500000001','1500000002')){Get-Api "/pickup-points/$point/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z"|ConvertTo-Json -Depth 5}

Get-Api '/marketplace/economics?from=2025-12-07T00:00:00Z&to=2026-01-06T00:00:00Z'|ConvertTo-Json -Depth 5

