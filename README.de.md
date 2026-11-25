# techo5-netbird

[English](README.md) | **Deutsch**

Mit diesem Add-on wird ein [TECHO5](https://github.com/HuskerMinion/techo5)-Gerät Teil deines eigenen
[NetBird](https://netbird.io)-Netzes. So kann auch ein Home Assistant an einem anderen Standort darauf
zugreifen. Du kannst NetBirds gehosteten Dienst nutzen oder einen eigenen Management-Server.

Das Add-on verwendet NetBirds offiziellen Client. Er liegt auf der userdata-Partition und wird bei
jedem Systemstart über TECHO5s
[rc.local](https://github.com/HuskerMinion/techo5/blob/main/docs/setup.md#running-something-of-your-own-at-boot)
gestartet. Deshalb bleibt er auch nach TECHO5-Updates erhalten. Das Add-on ist ein eigenständiges
Projekt: TECHO5 pflegt und testet es nicht.

## Was es tut und was nicht

Es betreibt NetBird genauso, wie es das in TECHO5 eingebaute NetBird bisher getan hat:

- **Home Assistant über NetBird.** Home Assistant verbindet sich über die verschlüsselte
  ESPHome-Schnittstelle (Port 6053) mit dem Gerät, unter dessen NetBird-Namen oder -Adresse.
- **Sonst kommt nichts durch.** TECHO5s eigene Firewall erlaubt auf NetBirds Netzwerkschnittstelle
  (`netbird0`) nur Ping und die Ports 6053, 8181 (Setup-Seite, nur bei eingeschaltetem Remote-Setup)
  und 22 (SSH, wenn aktiviert). Ohne diese Firewall-Regel startet das Add-on NetBird gar nicht erst.
  Mit NetBirds Access-Policies lässt sich der Zugriff pro Peer weiter einschränken, aber nie erweitern.
- **Das Gerät übernimmt nichts aus dem Netz.** Kein DNS, kein IPv6, kein NetBird-SSH-Server und keine
  Routen, solange du keine Netze freigibst (siehe [Routen aus dem Mesh](#routen-aus-dem-mesh)). Es
  stellt selbst keine Routen bereit und ist kein Exit-Node. Der Client sendet keine Metriken an NetBird.
- **NetBirds Client im Original.** Das Add-on lädt NetBirds offizielles Release-Archiv für arm64 und
  prüft vor dem Entpacken Größe und SHA-256 gegen die Werte, die in diesem Release festgeschrieben
  sind. Warum es keinen eigenen Build gibt, steht in [docs/client-binary.md](docs/client-binary.md)
  (auf Englisch).

In Home Assistant und auf dem Display taucht das Add-on nicht auf. Eingerichtet und überprüft wird es
per SSH.

## Geräte

Alle TECHO5-Geräte, auf denen das eingebaute NetBird lief und die beim Start `rc.local` ausführen: die
Echo Shows und der Echo Spot. Ein und dasselbe arm64-Programm läuft auf allen, auch auf dem Show 8
(2. Generation) mit seinem 32-Bit-Userland.

**Getestet auf:** einem Echo Show 8 (2. Generation). Der Echo Spot sollte genauso funktionieren, ist
aber noch nicht getestet. Für die Echo Dots und den Skylight ist das Add-on nicht gedacht.

Auf diesem Gerät geprüft, mit einem selbst betriebenen Management-Server:

- Installation und Übernahme (`migrate`) des Peers aus TECHO5s eingebautem NetBird: Name und Adresse
  bleiben gleich, das Profil bleibt unverändert.
- Beitritt (`join`) mit einem neuen Setup-Key: Ein neuer Peer entsteht und verbindet sich. Der Key
  landet in keiner Datei und nicht im Log.
- Über das Mesh kommen Ping und SSH an, alles andere blockieren Firewall oder Policies.
- Fehlt die Firewall-Regel oder ist TECHO5s NetBird eingeschaltet, stoppt NetBird. Ist das behoben,
  läuft es wieder.
- Nach dem Beenden von NetBird oder des Add-ons und nach einem Neustart des Geräts startet alles von
  selbst neu. Rund 20 Sekunden nach dem Booten ist das Gerät verbunden.
- Routen: Mit freigegebenem Heimnetz war ein Rechner dort über `netbird0` erreichbar. War nur ein
  anderes Netz freigegeben, wurde ein von Hand ausgewähltes Heimnetz nach wenigen Sekunden wieder
  abgewählt. `--clear` entfernte die Route. Zu große Netze, IPv6 und die eigenen Netze des Geräts
  wurden abgelehnt.
- TECHO5-Update (von v1.6.45 auf v1.6.46) bei laufendem Add-on: Nach dem Neustart startet `rc.local`
  das Add-on wieder, es ist binnen 20 Sekunden verbunden. Peer, Einstellungen und Firewall-Regel
  bleiben unverändert.
- `tools/install.py`: Neuinstallation mit Übernahme des Peers und Update eines verbundenen Add-ons.
- Update auf TECHO5 v1.6.47, das das eingebaute NetBird entfernt: Peer und Management-Server bleiben
  für `migrate` erhalten, danach ist derselbe Peer mit gleichem Namen und gleicher Adresse wieder da.

### Getestete TECHO5-Versionen

Alle auf dem Echo Show 8 (2. Generation):

| TECHO5 | Add-on | Getestet |
|---|---|---|
| v1.6.45 | v0.1.0 (Test-Builds) | Installation, `migrate`, `join` mit neuem Setup-Key, Policies, Firewall- und Konfliktprüfung, Neustarts und Reboot, Routen aus dem Mesh, `tools/install.py` |
| v1.6.45 → v1.6.46 | v0.1.0 | TECHO5-Update bei laufendem Add-on: über `rc.local` wieder gestartet und verbunden, Peer, Einstellungen und Firewall-Regel unverändert |
| v1.6.46 → v1.6.47 | v0.1.0 | Entfernen des eingebauten NetBird beim Update, danach `tools/install.py` mit `migrate`: derselbe Peer, Policies, ein Reboot |
| v1.6.47 | v0.1.1 | Update des Add-ons über v0.1.0: Peer bleibt, keine Frage nach einem NetBird-Schalter |

Versionen vor v1.6.45 sind nicht getestet.

## Voraussetzungen

- TECHO5 mit aktiviertem SSH und hinterlegtem Schlüssel (in den Einstellungen auf dem Gerät oder über
  die Setup-Seite).
- Ein **Verschlüsselungsschlüssel** für die Verbindung zwischen Home Assistant und dem Gerät. Ohne ihn
  startet das Add-on NetBird nicht, denn sonst liefe die Verbindung über NetBird unverschlüsselt.
- Bis TECHO5 v1.6.46 ist das eingebaute NetBird **aus**. Ab v1.6.47 gibt es keins mehr: Das Update
  schaltet es ab und hebt seinen Peer für `migrate` auf.
- Tailscale ist aus. NetBird und Tailscale vergeben beide Adressen aus 100.64.0.0/10 und können nicht
  gleichzeitig laufen.
- Etwa 60 MB freier Speicher auf userdata und rund 110 MB Arbeitsspeicher im Betrieb.

## Installation

Es gibt zwei Wege: über TECHO5s Karte **Add-ons**, dann installiert, startet und aktualisiert TECHO5 das
Add-on, oder mit `tools/install.py` von deinem Rechner aus, dann startet es über `rc.local`. Nimm einen
der beiden, nicht beide: Eine Installation über die Karte übernimmt eine von Hand.

### Über TECHO5s Karte „Add-ons“

Ab TECHO5 v1.6.53, sobald TECHO5s Add-on-Katalog dieses Add-on aufführt: Auf der Setup-Seite des Geräts,
Reiter **System**, Karte **Add-ons**, **techo5-netbird** installieren. Die Karte warnt vor jeder
Installation und sagt fett, dass es als root läuft (NetBird legt eine Netzwerkschnittstelle an).

TECHO5 lädt das Add-on und NetBirds Client-Archiv, die der signierte Katalog per SHA-256 festlegt (das
Add-on prüft das Archiv noch einmal gegen seinen eigenen Wert), installiert beides unter
`/data/techo5-addons/techo5-netbird`, startet es, hält es am Laufen und bietet Updates auf der Karte
und in Home Assistant an. Das Log steht auf der Karte.

Beim ersten Start übernimmt es einen Peer, solange es noch keinen eigenen hat. Ein neuer Setup-Key ist
dann nicht nötig:

- **eine Installation von Hand** dieses Add-ons: Es nimmt sie aus `rc.local`, stoppt sie, übernimmt
  ihren Peer und ihre Einstellungen (Management-Server, Routen) und löscht `/data/techo5-netbird`;
- sonst den Peer von **TECHO5s eingebautem NetBird**, den TECHO5 v1.6.47 dafür aufgehoben hat.

Gibt es beides nicht, trittst du per SSH bei, wie unter [Neues Gerät](#neues-gerät):

```sh
ssh -t root@<gerät> /data/techo5-addons/techo5-netbird/bin/techo5-netbird join --management https://netbird.example.com
```

Die anderen Befehle unten (`status`, `routes`) gehen per SSH genauso, als
`/data/techo5-addons/techo5-netbird/bin/techo5-netbird`. Entfernt wird es auf der Karte: Der Peer
bleibt für eine neue Installation erhalten, außer du setzt das Häkchen, ihn mitzulöschen.

### Mit tools/install.py

Auf deinem Rechner, mit Python 3 und einem SSH-Client (ab Windows 10 ist einer dabei):

```sh
git clone https://github.com/tioan/techo5-netbird
cd techo5-netbird
python3 tools/install.py <gerät>
```

`<gerät>` ist die Adresse oder der Name des Geräts in deinem Netz. Das Skript:

1. lädt `techo5-netbird-arm64` aus dem [neuesten Release](https://github.com/tioan/techo5-netbird/releases/latest)
   und prüft es gegen die `SHA256SUMS` des Release,
2. überträgt es per SSH aufs Gerät und installiert es dort: Das Add-on kopiert sich nach
   `/data/techo5-netbird`, lädt NetBirds Client (etwa 15 MB, entpackt 40 MB), trägt eine markierte
   Zeile in `/data/techo5-linux/rc.local` ein (der Rest der Datei bleibt unverändert) und startet,
3. fragt, wie das Gerät zu seinem Peer kommt: Übernahme aus TECHO5s eingebautem NetBird oder Beitritt
   mit einem neuen Setup-Key (beides unten beschrieben).

Mit `--migrate`, `--join [--management URL]` oder `--skip` beantwortest du die Frage vorab.
`--version v0.1.0` installiert ein bestimmtes Release, `--file` ein vorhandenes Programm, und
`--dry-run` lädt nur herunter und prüft. Alle Optionen zeigt `python3 tools/install.py --help`.

Der SHA-256 stammt aus demselben Release wie das Programm. Er erkennt also einen fehlerhaften Download,
aber kein Release, das nicht von diesem Projekt stammt.

### Gerät mit TECHO5s eingebautem NetBird

Der bisherige Peer bleibt erhalten, ein neuer Setup-Key ist nicht nötig: Bestätige die Übernahme, wenn
das Skript sie anbietet. Das klappt vor und nach dem Update auf TECHO5 v1.6.47 gleich, denn v1.6.47
entfernt zwar das eingebaute NetBird, hebt aber seinen Peer auf.

Das Skript führt dann `migrate` auf dem Gerät aus. Dabei wandert die Identität des Peers samt
Management-Server aus TECHO5s NetBird-Ordner in den des Add-ons.

- **Bis v1.6.46** muss TECHO5s **NetBird** vorher aus sein. Ist es noch an, fordert dich das Skript
  auf, es auszuschalten (auf dem Display oder in Home Assistant). Lass es danach aus: Eingeschaltet
  bräuchte es einen neuen Key, und das Add-on hält NetBird an, solange es an ist.
- **Ab v1.6.47** gibt es keinen Schalter mehr. Home Assistant zeigt den alten NetBird-Schalter und
  -Status als nicht verfügbar an; du kannst beide dort löschen.

### Neues Gerät

1. **Setup-Key anlegen** im NetBird-Dashboard unter **Setup Keys → Create Setup Key**. Am besten
   einmalig verwendbar, kurz gültig und mit einer Gruppe (etwa `techo5`) für deine Access-Policies.
2. **Beitreten:** Bestätige den Beitritt, wenn das Skript ihn anbietet, und gib deinen
   Management-Server an (leer lassen für NetBirds gehosteten Dienst). Den Key fragt das Gerät selbst
   in deinem Terminal ab, die Eingabe bleibt unsichtbar. Er wird nur einmal zum Beitritt verwendet und
   nirgends gespeichert: nicht in einer Datei, nicht als Argument, nicht im Log. Auch das Skript
   bekommt ihn nie zu sehen.
3. **Gerät in Home Assistant unter seinem NetBird-Namen hinzufügen**, also als
   `<name>.<deine Peer-Domain>`, mit demselben Verschlüsselungsschlüssel. Den alten Eintrag mit der
   Heimnetz-Adresse entfernst du.

### Von Hand

Die Befehle in dieser Anleitung stehen als `techo5-netbird` da. Auf dem Gerät rufst du sie als
`/data/techo5-netbird/bin/techo5-netbird` auf. Ohne das Skript sieht die Installation so aus:

```sh
ssh root@<gerät> 'cat > /tmp/techo5-netbird && chmod 755 /tmp/techo5-netbird && /tmp/techo5-netbird install && rm /tmp/techo5-netbird' < techo5-netbird-arm64
ssh root@<gerät> /data/techo5-netbird/bin/techo5-netbird migrate      # oder:
ssh -t root@<gerät> /data/techo5-netbird/bin/techo5-netbird join --management https://netbird.example.com
```

## Access-Policies

In einem neuen NetBird-Netz kann jeder Peer jeden anderen erreichen. Erlaube der Gruppe deines
Home-Assistant-Hosts nur TCP 6053 zur Gruppe `techo5`, und nur deinen eigenen Geräten TCP 8181 und 22.
Die Firewall des Clients ist aktiv und setzt diese Regeln durch. TECHO5s Firewall bleibt dabei die
äußere Grenze.

## Routen aus dem Mesh

Standardmäßig aus. Sobald du Netze freigibst, darf das Gerät die Routen nutzen, die ihm das Mesh
anbietet, etwa ins Heimnetz hinter einem Routing-Peer. Aber nur in die freigegebenen Netze:

```sh
techo5-netbird routes 192.168.1.0/24            # nur dieses Netz
techo5-netbird routes 192.168.1.0/24 10.20.0.0/16
techo5-netbird routes                           # freigegebene Netze anzeigen
techo5-netbird routes --clear                   # keine Routen mehr
```

Erlaubt sind nur IPv4-Netze, höchstens ein /8, und keine, die sich mit den eigenen Netzen des Geräts
überschneiden. NetBird selbst würde jede Route übernehmen, die der Management-Server anbietet, auch
später hinzugekommene. Das Add-on begrenzt die Auswahl auf die freigegebenen Netze und korrigiert sie
innerhalb von 15 Sekunden, falls eine weitere Route auftaucht. Beim Ein- oder Ausschalten der Routen
startet NetBird neu. Das Gerät routet nie für andere und nutzt keinen Exit-Node.

## Im Alltag

```sh
techo5-netbird status
```

Zeigt, ob das Add-on beim Booten startet, ob NetBird läuft und verbunden ist, unter welchem Namen und
welcher Adresse, welche Routen genutzt werden und was gegebenenfalls nicht stimmt. Das Log liegt unter
`/data/techo5-netbird/techo5-netbird.log` und enthält auch NetBirds eigene Meldungen. Ab 1 MB wird es
als `.1` gesichert und neu begonnen.

Beendet sich NetBird, startet das Add-on es neu: zuerst nach 5 Sekunden, danach in wachsenden Abständen
bis zu 5 Minuten. Schlägt eine Prüfung fehl (Firewall-Regel fehlt, kein Verschlüsselungsschlüssel,
TECHO5s NetBird oder Tailscale eingeschaltet), wartet es und prüft jede Minute erneut.

## Updates

- **TECHO5-Updates** betreffen das Add-on nicht: Es liegt auf userdata, und TECHO5 (oder `rc.local`)
  startet es wieder.
- **Add-on-Updates über die Karte:** auf der Karte „Add-ons“ oder über die Update-Entity des Add-ons in
  Home Assistant.
- **Add-on-Updates von Hand:** Nach einem `git pull` einfach `python3 tools/install.py <gerät>` erneut
  ausführen. Die neue Version ersetzt die alte und startet neu, Peer und Einstellungen bleiben
  erhalten. Legt ein Release eine neuere NetBird-Version fest, wird sie heruntergeladen und die alte
  entfernt. Was sich geändert hat, steht in [CHANGELOG.md](CHANGELOG.md) (auf Englisch).

## Deinstallation

Über die Karte installiert: **Entfernen** auf der Karte. Von Hand:

```sh
techo5-netbird uninstall           # stoppt das Add-on und entfernt es aus dem Autostart; der Peer bleibt
techo5-netbird uninstall --purge   # löscht zusätzlich /data/techo5-netbird und damit die Identität des Peers
```

Entferne das Gerät anschließend unter **Peers** im NetBird-Dashboard. Alles andere in `rc.local`
bleibt erhalten.

## Gut zu wissen

- **Zurücksetzen für einen neuen Besitzer** (in TECHO5s Einstellungen) löscht weder das Add-on noch
  seinen Peer. Führe vorher `techo5-netbird uninstall --purge` aus.
- **Wer NetBird aus der Ferne abschaltet**, sperrt sich selbst aus. Das gilt auch, wenn du das Add-on
  per SSH über NetBird stoppst.
- **Diagnosepakete** von TECHO5 kennen das Add-on nicht und enthalten seinen NetBird-Namen und seine
  Adresse im Klartext. Prüfe sie, bevor du sie weitergibst.
- **Last auf dem Gerät:** Auf einem Echo Show 8 (2. Generation) brauchte NetBird im Leerlauf etwa
  1,5 % eines Kerns, bei rund 50 Paketen pro Sekunde etwa 4,5 %, dazu um die 110 MB Arbeitsspeicher.

## Selbst bauen

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=<version>" -o techo5-netbird-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags "-s -w -X main.version=<version>" -o techo5-netbird-arm .
```

`arm64` nutzt die Installation von Hand. `arm` gibt TECHO5s Add-on-Katalog jedem Gerät, weil TECHO5s
eigene Programme 32-bit sind; es braucht den 64-Bit-Kernel der Echo Shows und des Echo Spot für NetBirds
`linux_arm64`-Client. Jedes Release enthält beide, `addon.json` (was der Katalog aufführt),
`SHA256SUMS` und einen Herkunftsnachweis des Builds (`gh attestation verify <datei> -R tioan/techo5-netbird`).

Die Go-Tests laufen nur unter Linux: `CGO_ENABLED=0 go test ./...`. Die Tests des Installers laufen
überall: `python3 -m unittest discover -s tools`.

## Lizenz

MIT, siehe [LICENSE](LICENSE). NetBirds Client ist weder Teil dieses Repositorys noch seiner Releases:
Das Gerät lädt ihn direkt aus NetBirds offiziellem Release, zusammen mit NetBirds Lizenzen, die das
Add-on daneben ablegt (BSD-3-Clause für den Client). Details in [NOTICE](NOTICE).

Einige Teile stammen aus TECHO5 (MIT), jeweils im Quelltext vermerkt.
