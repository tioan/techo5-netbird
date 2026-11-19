# techo5-netbird

[English](README.md) | **Deutsch**

Bringt ein [TECHO5](https://github.com/HuskerMinion/techo5)-Gerät in dein eigenes
[NetBird](https://netbird.io)-Netz, damit ein Home Assistant an einem anderen Ort es erreicht. Das geht
mit NetBirds eigenem Dienst oder mit einem Management-Server, den du selbst betreibst.

Es ist NetBirds eigener Client. Er läuft von userdata und wird bei jedem Start über TECHO5s
[rc.local](https://github.com/HuskerMinion/techo5/blob/main/docs/setup.md#running-something-of-your-own-at-boot)
gestartet, deshalb übersteht er TECHO5-Updates. Das Add-on wird von seinem Autor gebaut und gepflegt,
nicht von TECHO5, und TECHO5 testet es nicht.

## Was es macht und was nicht

Es betreibt NetBird so, wie es TECHO5s eingebautes NetBird getan hat:

- **Home Assistant über NetBird.** Home Assistant erreicht das Gerät über seine verschlüsselte
  ESPHome-Verbindung (Port 6053), unter seinem NetBird-Namen oder seiner NetBird-Adresse.
- **Sonst kommt nichts herein.** TECHO5s eigene Firewall lässt über NetBirds Interface (`netbird0`)
  nur die Ports 6053, 8181 (die Setup-Seite, nur solange Remote-Setup an ist) und 22 (SSH, falls du es
  eingeschaltet hast) sowie Ping zu. Das Add-on startet NetBird nur, wenn diese Firewall-Regel vorhanden
  ist. NetBirds Access-Policies schränken das pro Peer weiter ein; eine Policy kann nie mehr öffnen.
- **Das Gerät übernimmt nichts aus dem Netz.** Kein DNS, kein IPv6, kein NetBird-SSH-Server, keine
  Routen (außer du gibst Netze an: siehe [Routen aus dem Mesh](#routen-aus-dem-mesh)). Es routet nichts
  Eigenes ins Mesh und ist kein Exit-Node. Der Client schickt NetBird keine Metriken.
- **NetBirds Client, so wie NetBird ihn veröffentlicht.** Das Add-on lädt NetBirds eigenes
  Release-Archiv für arm64 herunter und prüft Größe und SHA-256 gegen die in diesem Release
  festgelegten Werte, bevor es das Archiv entpackt. Warum es keinen eigenen Build verwendet:
  [docs/client-binary.md](docs/client-binary.md) (Englisch).

In Home Assistant gibt es keinen Schalter und keinen Status dafür und auf dem Bildschirm nichts:
Einrichtung und Kontrolle laufen über SSH.

## Geräte

Jedes TECHO5-Gerät, auf dem TECHO5s eingebautes NetBird lief und das beim Start `rc.local` ausführt:
die Echo Shows und der Echo Spot. Ein einziges arm64-Programm läuft auf allen, auch auf dem Show 8
(2. Gen.) mit seinem 32-Bit-Userland.

**Getestet auf:** einem Echo Show 8 (2. Gen.). Der Echo Spot sollte genauso funktionieren, ist aber
nicht getestet. Nicht für die Echo Dots und den Skylight.

## Was du brauchst

- TECHO5 mit eingeschaltetem SSH und hinterlegtem Schlüssel (Einstellungen auf dem Gerät oder die
  Setup-Seite).
- Einen **Verschlüsselungsschlüssel** auf der Verbindung von Home Assistant zum Gerät. Ohne ihn startet
  das Add-on NetBird nicht: Über NetBird liefe die Verbindung sonst im Klartext.
- TECHO5s eigenes NetBird **aus** und Tailscale aus. NetBird und Tailscale nummerieren ihre Geräte
  beide in 100.64.0.0/10 und können nicht nebeneinander laufen.
- Etwa 60 MB frei auf userdata und rund 110 MB Arbeitsspeicher im Betrieb.

## Installation

Lade `techo5-netbird` aus dem [neuesten Release](https://github.com/tioan/techo5-netbird/releases/latest)
herunter und vergleiche seinen SHA-256 mit dem auf der Release-Seite. Dann, von deinem Rechner aus:

```sh
ssh root@<device> 'cat > /tmp/techo5-netbird && chmod 755 /tmp/techo5-netbird && /tmp/techo5-netbird install && rm /tmp/techo5-netbird' < techo5-netbird
```

`install` kopiert sich nach `/data/techo5-netbird/bin`, lädt NetBirds Client herunter (etwa 15 MB,
entpackt 40 MB), fügt eine markierte Zeile in `/data/techo5-linux/rc.local` ein (alles andere in der
Datei bleibt, wie es ist) und startet das Add-on.

### Ein Gerät, das schon TECHO5s eingebautes NetBird nutzt

Derselbe Peer bleibt, ohne neuen Setup-Key:

1. Schalte TECHO5s **NetBird**-Schalter aus, auf dem Bildschirm oder in Home Assistant.
2. Installiere das Add-on wie oben.
3. `ssh root@<device> /data/techo5-netbird/bin/techo5-netbird migrate`

Die Befehle unten stehen als `techo5-netbird` da: Führe sie auf dem Gerät als
`/data/techo5-netbird/bin/techo5-netbird` aus.

`migrate` verschiebt die Identität des Peers aus TECHO5s NetBird-Ordner in den des Add-ons, zusammen
mit dem Management-Server, bei dem er angemeldet war. Lass TECHO5s NetBird-Schalter danach aus:
Eingeschaltet bräuchte er einen neuen Key, und das Add-on stoppt NetBird, solange er an ist.

### Ein neues Gerät

1. **Erstelle einen Setup-Key** in deinem NetBird-Dashboard: **Setup Keys → Create Setup Key**. Mach
   ihn einmalig, mit kurzer Gültigkeit, und gib ihm eine Gruppe (etwa `techo5`) für deine
   Access-Policies.
2. **Anmelden:**

   ```sh
   ssh -t root@<device> /data/techo5-netbird/bin/techo5-netbird join --management https://netbird.example.com
   ```

   Lass `--management` für NetBirds eigenen Dienst weg. Der Key wird im Terminal abgefragt und beim
   Tippen nicht angezeigt; er kann auch über stdin kommen. Er wird einmal zum Anmelden benutzt und
   nicht aufbewahrt: nicht in einer Datei, nicht als Argument, nicht im Log.
3. **Füge das Gerät in Home Assistant unter seinem NetBird-Namen hinzu**, als
   `<name>.<deine Peer-Domain>`, mit demselben Verschlüsselungsschlüssel, und entferne den alten
   Eintrag mit der Heimadresse.

## Access-Policies

Ein neues NetBird-Netz lässt jeden Peer jeden anderen erreichen. Erlaube nur der Gruppe deines
Home-Assistant-Hosts den Zugriff auf die Gruppe `techo5` über TCP 6053 und nur deinen eigenen Geräten
TCP 8181 und 22. Die Firewall des Clients ist an und setzt das durch, unterhalb von TECHO5s Firewall,
die die Obergrenze bleibt.

## Routen aus dem Mesh

Aus, bis du Netze angibst. Dann darf das Gerät die Routen nutzen, die das Mesh ihm anbietet, etwa das
LAN zu Hause hinter einem Routing-Peer, aber nur zu den angegebenen Netzen:

```sh
techo5-netbird routes 192.168.1.0/24            # nur dieses Netz
techo5-netbird routes 192.168.1.0/24 10.20.0.0/16
techo5-netbird routes                           # was angegeben ist
techo5-netbird routes --clear                   # keine: gar keine Routen
```

Nur IPv4-Netze, höchstens /8, und keine, die sich mit den eigenen Netzen des Geräts überschneiden.
NetBird würde jede Route übernehmen, die sein Management-Server anbietet, auch später hinzugefügte. Das
Add-on beschränkt die Auswahl auf die angegebenen Netze und wählt innerhalb von 15 Sekunden neu aus,
wenn eine andere Route auftaucht. Routen ein- oder auszuschalten startet NetBird neu. Das Gerät routet
nie für andere und nutzt keinen Exit-Node.

## Im Alltag

```sh
techo5-netbird status
```

Zeigt, ob das Add-on beim Start gestartet wird, ob NetBird läuft und verbunden ist, unter welchem Namen
und welcher Adresse, die genutzten Routen und was nicht stimmt, falls etwas nicht stimmt. Das Log ist
`/data/techo5-netbird/techo5-netbird.log`, NetBirds eigene Zeilen eingeschlossen. Bei 1 MB wird es als
`.1` aufbewahrt und ein neues begonnen.

Das Add-on startet NetBird neu, wenn es sich beendet: zuerst nach 5 Sekunden, dann mit bis zu 5 Minuten
Abstand. Es wartet, solange eine Prüfung fehlschlägt (keine Firewall-Regel, kein
Verschlüsselungsschlüssel, TECHO5s NetBird oder Tailscale an), und schaut jede Minute erneut nach.

## Updates

- **TECHO5-Updates** lassen das Add-on in Ruhe: Es liegt auf userdata, und `rc.local` startet es
  wieder.
- **Add-on-Updates:** Installiere das neue Release genauso wie unter [Installation](#installation). Es
  ersetzt das alte und startet neu; Peer und Einstellungen bleiben erhalten. Ein Release, das ein
  neueres NetBird festlegt, lädt es herunter und entfernt das alte.

## Deinstallation

```sh
techo5-netbird uninstall           # stoppt es, kein Start mehr beim Booten; der Peer bleibt
techo5-netbird uninstall --purge   # und löscht /data/techo5-netbird, samt Identität des Peers
```

Entferne das Gerät danach unter **Peers** in deinem NetBird-Dashboard. Alles andere in `rc.local`
bleibt erhalten.

## Gut zu wissen

- **Ein Zurücksetzen des Geräts für jemand anderen** (TECHO5s Einstellungen) löscht weder das Add-on
  noch seinen Peer. Führe vorher `techo5-netbird uninstall --purge` aus.
- **NetBird aus der Ferne auszuschalten** schneidet dir den Weg zum Gerät ab. Das Add-on per SSH über
  NetBird zu stoppen, ebenso.
- **Ein Diagnose-Paket** von TECHO5 kennt das Add-on nicht und verbirgt seinen NetBird-Namen und seine
  Adresse nicht. Lies es, bevor du es weitergibst.
- **Last auf dem Gerät:** Auf einem Echo Show 8 (2. Gen.) brauchte NetBird im Leerlauf etwa 1,5 % eines
  Kerns, mit rund 50 Paketen pro Sekunde etwa 4,5 %, und um die 110 MB Arbeitsspeicher.

## Bauen

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=<version>" -o techo5-netbird .
```

Die Tests laufen nur unter Linux: `CGO_ENABLED=0 go test ./...`.

## Lizenz

MIT, siehe [LICENSE](LICENSE). NetBirds Client ist weder Teil dieses Repositorys noch seiner Releases:
Das Gerät lädt ihn aus NetBirds eigenem Release herunter, mit NetBirds Lizenzen, die das Add-on daneben
ablegt (BSD-3-Clause für den Client). Siehe [NOTICE](NOTICE).

Teile sind aus TECHO5 (MIT) übernommen, wie im Quelltext vermerkt.
