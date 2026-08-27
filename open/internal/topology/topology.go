package topology

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type Game struct {
	ID    int
	Host  string
	Index int // 0-based position within the host's game list; port = BasePort + Index*1000
}

type Topology struct {
	games map[int]Game
}

func Load(gamesFile string) (*Topology, error) {
	games, err := loadGames(gamesFile)
	if err != nil {
		return nil, err
	}
	return &Topology{games: games}, nil
}

func (t *Topology) Game(id int) (Game, bool) {
	game, ok := t.games[id]
	return game, ok
}

func loadGames(path string) (map[int]Game, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open games file: %w", err)
	}
	defer file.Close()

	games := make(map[int]Game)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := stripComment(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected HOST [GAME_IDS]", path, lineNumber)
		}
		if net.ParseIP(fields[0]) == nil {
			return nil, fmt.Errorf("%s:%d: invalid host %q", path, lineNumber, fields[0])
		}
		ids, err := parseGameIDs(fields[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		for index, id := range ids {
			if previous, exists := games[id]; exists {
				return nil, fmt.Errorf("%s:%d: game%d is already assigned to %s", path, lineNumber, id, previous.Host)
			}
			games[id] = Game{ID: id, Host: fields[0], Index: index}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read games file: %w", err)
	}
	if len(games) == 0 {
		return nil, fmt.Errorf("%s contains no games", path)
	}
	return games, nil
}

func parseGameIDs(value string) ([]int, error) {
	if len(value) < 3 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, fmt.Errorf("invalid game list %q", value)
	}
	parts := strings.Split(value[1:len(value)-1], ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid game ID %q", part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func stripComment(line string) string {
	if index := strings.IndexByte(line, '#'); index >= 0 {
		line = line[:index]
	}
	return strings.TrimSpace(line)
}
