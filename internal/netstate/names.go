package netstate

import "math/rand"

var norse = []string{
	"Óðinn", "Þórr", "Freyja", "Loki", "Frigg", "Baldr", "Týr", "Heimdallr",
	"Freyr", "Njörðr", "Höðr", "Viðarr", "Váli", "Bragi", "Ullr", "Forseti",
	"Hœnir", "Hermóðr", "Kvasir", "Magni", "Móði", "Vili", "Vé", "Mímir",
	"Iðunn", "Sif", "Skaði", "Eir", "Gefjun", "Fulla", "Gná", "Sága",
	"Sjöfn", "Lofn", "Vör", "Vár", "Syn", "Hlín", "Snotra", "Sól",
	"Bil", "Jörð", "Rindr", "Nanna", "Sigyn", "Ægir", "Rán", "Hel",
	"Fenrir", "Fenrisúlfr", "Jörmungandr", "Miðgarðsormr", "Surtr", "Ymir",
	"Hræsvelgr", "Útgarða-Loki", "Skrýmir", "Baugi", "Suttungr", "Vafþrúðnir",
	"Þjazi", "Garmr", "Níðhöggr", "Völundr", "Dvalinn", "Durinn", "Alvíss",
	"Andvari", "Brokkr", "Eitri", "Sindri", "Fjalarr", "Galarr", "Fafnir",
	"Reginn", "Urðr", "Verðandi", "Skuld", "Brynhildr", "Sigrdrífa", "Göll",
	"Herfjöturr", "Hildr", "Hlökk", "Radgríðr", "Sleipnir", "Huginn", "Muninn",
	"Geri", "Freki", "Tanngnjóstr", "Tanngrisnir", "Gullinbursti", "Heiðrún",
	"Ratatoskr", "Veðrfölnir",
}

var greek = []string{
	"Zeus", "Hēra", "Poseidōn", "Dēmētēr", "Athēna", "Apollōn", "Artemis",
	"Arēs", "Aphroditē", "Hēphaistos", "Hermēs", "Hestia", "Dionysos", "Haidēs",
	"Persephonē", "Kronos", "Rhea", "Ōkeanos", "Tēthys", "Hyperiōn", "Theia",
	"Iapetos", "Koios", "Phoibē", "Mnēmosynē", "Themis", "Krios", "Atlas",
	"Promētheus", "Epimētheus", "Chaos", "Gaia", "Ouranos", "Erebos", "Nyx",
	"Tartaros", "Erōs", "Pan", "Hekatē", "Hēlios", "Selēnē", "Ēōs", "Iris",
	"Hēbē", "Enyō", "Phobos", "Deimos", "Eris", "Nemesis", "Tychē", "Morpheus",
	"Thanatos", "Hypnos", "Charōn", "Minōs", "Rhadamanthys", "Aiakos",
	"Kerberos", "Chimaira", "Hydra", "Sphinx", "Medousa", "Pēgasos",
	"Kentauros", "Minōtauros", "Polyphēmos", "Tritōn", "Skylla", "Charybdis",
	"Ladōn", "Pythōn", "Typhōn", "Echidna", "Harpyia", "Seirēn", "Satyros",
	"Silēnos", "Nymphē", "Dryas",
}

func PoolFor(role Role) []string {
	if role == RoleEgress {
		return greek
	}
	return norse
}

func PickName(role Role, taken map[string]bool) string {
	pool := PoolFor(role)

	free := make([]string, 0, len(pool))
	for _, name := range pool {
		if !taken[name] {
			free = append(free, name)
		}
	}
	if len(free) == 0 {
		return ""
	}
	return free[rand.Intn(len(free))]
}
