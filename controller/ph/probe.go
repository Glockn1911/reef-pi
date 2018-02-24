package ph

import (
	"encoding/json"
	"fmt"
	"github.com/reef-pi/drivers"
	"log"
	"math/rand"
	"time"
)

type Probe struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Address int           `json:"address"`
	Enable  bool          `json:"enable"`
	Period  time.Duration `json:"period"`
}

func (c *Controller) Get(id string) (Probe, error) {
	var p Probe
	return p, c.store.Get(Bucket, id, &p)
}

func (c Controller) List() ([]Probe, error) {
	probes := []Probe{}
	fn := func(v []byte) error {
		var p Probe
		if err := json.Unmarshal(v, &p); err != nil {
			return err
		}
		probes = append(probes, p)
		return nil
	}
	return probes, c.store.List(Bucket, fn)
}

func (c *Controller) Create(p Probe) error {
	if p.Period <= 0 {
		return fmt.Errorf("Period should be positive. Suppied:%f", p.Period)
	}
	fn := func(id string) interface{} {
		p.ID = id
		return &p
	}
	if err := c.store.Create(Bucket, fn); err != nil {
		return err
	}
	if p.Enable {
		quit := make(chan struct{})
		c.quitters[p.ID] = quit
		go c.Run(p, quit)
	}
	return nil
}

func (c *Controller) Update(id string, p Probe) error {
	p.ID = id
	if p.Period <= 0 {
		return fmt.Errorf("Period should be positive. Suppied:%f", p.Period)
	}
	if err := c.store.Update(Bucket, id, p); err != nil {
		return err
	}
	quit, ok := c.quitters[p.ID]
	if ok {
		close(quit)
		delete(c.quitters, p.ID)
	}
	if p.Enable {
		quit := make(chan struct{})
		c.quitters[p.ID] = quit
		go c.Run(p, quit)
	}
	return nil
}

func (c *Controller) Delete(id string) error {
	if err := c.store.Delete(Bucket, id); err != nil {
		return err
	}
	quit, ok := c.quitters[id]
	if ok {
		close(quit)
		delete(c.quitters, id)
	}
	return nil
}

func (c *Controller) Run(p Probe, quit chan struct{}) {
	if p.Period <= 0 {
		log.Printf("ERROR:ph sub-system. Invalid period set for probe:%s. Expected postive, found:%f\n", p.Name, p.Period)
		return
	}
	d := drivers.NewAtlasEZO(byte(p.Address), c.bus)
	ticker := time.NewTicker(p.Period * time.Second)
	for {
		select {
		case <-ticker.C:
			if c.config.DevMode {
				c.updateReadings(p.ID, 8+rand.Float64()*2)
				log.Println("ph subsysten: Running in devmode probe:", p.Name, "reading:", 10)
			} else {
				v, err := d.Read()
				if err != nil {
					log.Println("ph sub-system: ERROR: Failed to read probe:", p.Name, ". Error:", err)
					continue
				}
				c.updateReadings(p.ID, v)
				log.Println("ph sub-system: Probe:", p.Name, "Reading:", v)
			}
		case <-quit:
			ticker.Stop()
			return
		}
	}
}